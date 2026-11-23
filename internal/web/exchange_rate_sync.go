package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/exchangerate"
)

func (s *Server) exchangeRateSyncEnabled() bool {
	return s.cfg.SMBCExchangeRates && s.repository.Driver() == "postgres"
}

func (s *Server) RunExchangeRateSync(ctx context.Context) {
	if !s.exchangeRateSyncEnabled() {
		return
	}
	run := func() {
		org, err := s.repository.ExchangeRateOrganization(ctx, s.cfg.OrganizationCode)
		if err == nil {
			_, err = s.syncExchangeRates(ctx, org)
		}
		if err != nil && ctx.Err() == nil {
			s.log.Error("SMBC exchange rate synchronization", "error", err)
		}
	}
	run() // On startup, including weekends, recover the latest published rates.
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if exchangerate.Scheduled(now) {
				run()
			}
		}
	}
}

func (s *Server) syncExchangeRates(ctx context.Context, org string) (bool, error) {
	now := time.Now().UTC()
	claimed, err := s.repository.ClaimExchangeRateSync(ctx, org, now)
	if err != nil || !claimed {
		return false, err
	}
	state, err := s.repository.ExchangeRateSyncStatus(ctx, org)
	if err != nil {
		return false, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 22*time.Second)
	defer cancel()
	p, err := exchangerate.NewFetcher().Fetch(fetchCtx, now, state.PublishedDate)
	if err == nil {
		err = s.repository.CompleteExchangeRateSync(ctx, org, p, time.Now().UTC())
	}
	if err != nil {
		// Cancellation on shutdown must not create a false outage notification.
		if ctx.Err() == nil {
			if failureErr := s.repository.FailExchangeRateSync(ctx, org, time.Now().UTC()); failureErr != nil {
				s.log.Error("record exchange rate failure", "error", failureErr)
			}
		}
		return false, err
	}
	return !p.Unchanged, nil
}

func (s *Server) apiExchangeRateSync(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "SMBC仲値の取得")
	if !ok {
		return
	}
	if !s.exchangeRateSyncEnabled() {
		writeAPIError(w, http.StatusConflict, "sync_disabled", "SMBC自動取得は有効になっていません。")
		return
	}
	configuredOrg, err := s.repository.ExchangeRateOrganization(r.Context(), s.cfg.OrganizationCode)
	if err != nil || configuredOrg != user.OrganizationID {
		writeAPIError(w, http.StatusForbidden, "sync_forbidden", "この組織では取得できません。")
		return
	}
	updated, err := s.syncExchangeRates(r.Context(), user.OrganizationID)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.log.Error("manual SMBC sync", "error", err)
		}
		writeAPIError(w, http.StatusBadGateway, "sync_failed", "SMBCの仲値を取得できませんでした。直近の正常なレートは維持しています。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated})
}
