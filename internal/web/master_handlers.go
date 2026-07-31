package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type masterCategory struct {
	Key   string
	Name  string
	Icon  string
	Count int
}

type masterExchangeRateCard struct {
	Currency  string
	Label     string
	Rate      string
	Date      string
	Updater   string
	Available bool
}

var masterCategoryCatalog = []masterCategory{
	{Key: "brands", Name: "ブランド名", Icon: "◆"},
	{Key: "suppliers", Name: "仕入先", Icon: "◩"},
	{Key: "buyers", Name: "仕入担当者", Icon: "♟"},
	{Key: "materials", Name: "素材", Icon: "◇"},
	{Key: "movements", Name: "駆動方式", Icon: "⚙"},
	{Key: "sales-destinations", Name: "販売先", Icon: "▦"},
	{Key: "accessories", Name: "付属品", Icon: "▰"},
	{Key: "conditions", Name: "コンディション", Icon: "★"},
	{Key: "currencies", Name: "外貨レート", Icon: "◉"},
	{Key: "guests", Name: "ゲスト管理", Icon: "♣"},
	{Key: "passwords", Name: "パスワード管理", Icon: "●"},
	{Key: "partners", Name: "取引先会社", Icon: "▣"},
	{Key: "company", Name: "会社情報", Icon: "▥"},
	{Key: "dashboard", Name: "ダッシュボード管理", Icon: "◌"},
}

const visibleMasterCategoryCount = 10

func (s *Server) masters(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	selectedKey := strings.TrimSpace(r.URL.Query().Get("category"))
	if selectedKey == "" {
		selectedKey = "brands"
	}
	catalog := append([]masterCategory(nil), masterCategoryCatalog...)
	var selected masterCategory
	var selectedRecords []database.MasterRecord
	for index := range catalog {
		switch catalog[index].Key {
		case "currencies":
			catalog[index].Count = 4
		case "guests":
			catalog[index].Count = 10
		default:
			records, err := s.store.MasterRecords(r.Context(), user.OrganizationID, catalog[index].Key)
			if err != nil {
				http.Error(w, "マスタデータを取得できませんでした", http.StatusInternalServerError)
				return
			}
			catalog[index].Count = len(records)
			if catalog[index].Key == selectedKey {
				selectedRecords = records
			}
		}
		if catalog[index].Key == selectedKey {
			selected = catalog[index]
		}
	}
	if selected.Key == "" {
		http.NotFound(w, r)
		return
	}
	categories := append([]masterCategory(nil), catalog[:visibleMasterCategoryCount]...)
	if (selected.Key == "company" || selected.Key == "dashboard") && len(selectedRecords) == 0 {
		defaultName := "会社情報"
		if selected.Key == "dashboard" {
			defaultName = "ダッシュボード設定"
		}
		record, err := s.store.CreateMasterRecord(r.Context(), database.SaveMasterInput{
			OrganizationID: user.OrganizationID, Category: selected.Key, Name: defaultName,
			ActorUserID: user.ID,
		})
		if err != nil {
			http.Error(w, "初期設定を作成できませんでした", http.StatusInternalServerError)
			return
		}
		selectedRecords = []database.MasterRecord{record}
	}
	var masterUsers []database.User
	var guestCredentials []database.GuestCredential
	if selected.Key == "passwords" {
		var err error
		masterUsers, err = s.store.Users(r.Context(), user.OrganizationID)
		if err != nil {
			http.Error(w, "利用者情報を取得できませんでした", http.StatusInternalServerError)
			return
		}
		guestCredentials, err = s.store.GuestCredentials(r.Context(), user.OrganizationID)
		if err != nil {
			http.Error(w, "ゲスト認証情報を取得できませんでした", http.StatusInternalServerError)
			return
		}
	}
	var rates []masterExchangeRateCard
	if selected.Key == "currencies" {
		var err error
		rates, err = s.masterRateCards(r, user.OrganizationID)
		if err != nil {
			http.Error(w, "外貨レートを取得できませんでした", http.StatusInternalServerError)
			return
		}
	}
	var guestBoxes []database.GuestBox
	if selected.Key == "guests" {
		var err error
		guestBoxes, err = s.store.GuestBoxes(r.Context(), user.OrganizationID)
		if err != nil {
			http.Error(w, "BOX情報を取得できませんでした", http.StatusInternalServerError)
			return
		}
	}
	alertCount, err := s.pendingAlerts(r, user.OrganizationID)
	if err != nil {
		http.Error(w, "通知件数を取得できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "masters", http.StatusOK, pageData{
		Title: "マスタ登録", Active: "masters", User: user, CSRF: csrfFromRequest(r),
		MasterCategories: categories, MasterRecords: selectedRecords, MasterCategory: selected,
		MasterUsers: masterUsers, MasterGuestCredentials: guestCredentials, MasterExchangeRates: rates,
		GuestBoxes: guestBoxes,
		AlertCount: alertCount, Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	})
}

func (s *Server) masterRateCards(r *http.Request, organizationID string) ([]masterExchangeRateCard, error) {
	history, err := s.store.ExchangeRates(r.Context(), organizationID, 500)
	if err != nil {
		return nil, err
	}
	latest := make(map[string]database.ExchangeRate)
	for _, rate := range history {
		if rate.QuoteCurrency == "JPY" {
			if _, exists := latest[rate.BaseCurrency]; !exists {
				latest[rate.BaseCurrency] = rate
			}
		}
	}
	labels := map[string]string{
		"USD": "米ドル", "EUR": "ユーロ", "HKD": "香港ドル", "CHF": "スイスフラン",
	}
	defaults := map[string]string{"USD": "155", "EUR": "165", "HKD": "19.8", "CHF": "172"}
	cards := make([]masterExchangeRateCard, 0, 4)
	for _, currency := range []string{"USD", "EUR", "HKD", "CHF"} {
		card := masterExchangeRateCard{
			Currency: currency, Label: labels[currency], Rate: defaults[currency],
			Date:    time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02"),
			Updater: "管理者",
		}
		if rate, ok := latest[currency]; ok {
			card.Rate = formatRateForInput(rate.RateScaled, rate.Scale)
			card.Date = rate.ObservedAt.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
			card.Updater = rate.Provider
			card.Available = true
		}
		cards = append(cards, card)
	}
	return cards, nil
}

func formatRateForInput(rate, scale int64) string {
	if scale <= 0 {
		return ""
	}
	value := fmt.Sprintf("%.8f", float64(rate)/float64(scale))
	return strings.TrimRight(strings.TrimRight(value, "0"), ".")
}

func (s *Server) pendingAlerts(r *http.Request, organizationID string) (int, error) {
	requestGroups, err := s.store.PurchaseRequestGroups(r.Context(), organizationID, "pending")
	if err != nil {
		return 0, err
	}
	approvals, err := s.store.Approvals(r.Context(), organizationID)
	if err != nil {
		return 0, err
	}
	count := len(requestGroups)
	for _, approval := range approvals {
		if approval.Status == "pending" {
			count++
		}
	}
	return count, nil
}

func masterDetailsFromRequest(r *http.Request) map[string]string {
	keys := []string{
		"representative_name", "contact_person", "email", "phone", "notes",
		"bank_name", "branch_name", "account_number", "account_holder",
		"monthly_sales_target", "monthly_purchase_budget", "status", "approval_code",
	}
	details := make(map[string]string, len(keys))
	for _, key := range keys {
		if value := strings.TrimSpace(r.FormValue(key)); value != "" {
			details[key] = value
		}
	}
	return details
}

func masterInputFromRequest(r *http.Request, user database.User) database.SaveMasterInput {
	return database.SaveMasterInput{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		Category: r.FormValue("category"), Code: r.FormValue("code"), Name: r.FormValue("name"),
		Address: r.FormValue("address"), Contact: r.FormValue("contact"),
		InvoiceRegistrationNumber: r.FormValue("invoice_registration_number"),
		Details:                   masterDetailsFromRequest(r),
	}
}

func (s *Server) masterCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	input := masterInputFromRequest(r, user)
	record, err := s.store.CreateMasterRecord(r.Context(), input)
	if err != nil {
		s.redirectMasterResult(w, r, input.Category, "", masterErrorMessage(err))
		return
	}
	after, _ := json.Marshal(record)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "master_record",
		TargetID: record.ID, Action: "master.create", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	s.redirectMasterResult(w, r, input.Category, "マスタデータを追加しました。", "")
}

func (s *Server) masterUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	input := masterInputFromRequest(r, user)
	id := r.PathValue("id")
	if err := s.store.UpdateMasterRecord(r.Context(), id, input); err != nil {
		s.redirectMasterResult(w, r, input.Category, "", masterErrorMessage(err))
		return
	}
	after, _ := json.Marshal(input)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "master_record",
		TargetID: id, Action: "master.update", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	s.redirectMasterResult(w, r, input.Category, "マスタデータを更新しました。", "")
}

func (s *Server) masterDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	category := r.FormValue("category")
	id := r.PathValue("id")
	if err := s.store.DeleteMasterRecord(r.Context(), user.OrganizationID, category, id, user.ID); err != nil {
		s.redirectMasterResult(w, r, category, "", masterErrorMessage(err))
		return
	}
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "master_record",
		TargetID: id, Action: "master.delete", Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	s.redirectMasterResult(w, r, category, "マスタデータを削除しました。", "")
}

func (s *Server) masterExchangeRateUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	currency := strings.ToUpper(strings.TrimSpace(r.FormValue("currency")))
	if currency == "" {
		// Accept the field name used by the first Phase 8 form revision as well.
		currency = strings.ToUpper(strings.TrimSpace(r.FormValue("base_currency")))
	}
	rateScaled, err := database.ParseRate(r.FormValue("rate"))
	if err != nil {
		s.redirectMasterResult(w, r, "currencies", "", err.Error())
		return
	}
	observedAt := strings.TrimSpace(r.FormValue("observed_at"))
	if len(observedAt) == 10 {
		observedAt += "T12:00"
	}
	rate, err := s.store.AddExchangeRate(r.Context(), user.OrganizationID, currency, "JPY",
		rateScaled, user.DisplayName, observedAt, user.ID)
	if err != nil {
		s.redirectMasterResult(w, r, "currencies", "", err.Error())
		return
	}
	after, _ := json.Marshal(rate)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "exchange_rate",
		TargetID: rate.ID, Action: "exchange_rate.updated", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	s.redirectMasterResult(w, r, "currencies", currency+"レートを保存しました。", "")
}

func (s *Server) redirectMasterResult(w http.ResponseWriter, r *http.Request, category, notice, errorMessage string) {
	values := url.Values{"category": {category}}
	if notice != "" {
		values.Set("notice", notice)
	}
	if errorMessage != "" {
		values.Set("error", errorMessage)
	}
	http.Redirect(w, r, "/masters?"+values.Encode(), http.StatusSeeOther)
}

func masterErrorMessage(err error) string {
	switch {
	case errors.Is(err, database.ErrMasterCodeExists):
		return "同じコードが既に登録されています。"
	case errors.Is(err, database.ErrInvalidMasterCategory):
		return "マスタ分類が正しくありません。"
	case errors.Is(err, database.ErrMasterRecordNotFound):
		return "対象のマスタデータが見つかりません。"
	default:
		return err.Error()
	}
}

func (s *Server) masterPasswordUserCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	created, err := s.store.CreateManagedUser(r.Context(), database.ManagedUserInput{
		OrganizationID: user.OrganizationID,
		Username:       r.FormValue("username"),
		DisplayName:    r.FormValue("display_name"),
		Role:           r.FormValue("role"),
		Password:       r.FormValue("password"),
	})
	if err != nil {
		s.redirectMasterResult(w, r, "passwords", "", masterPasswordError(err))
		return
	}
	s.auditTransaction(r, user, "user", created.ID, "user.created",
		map[string]string{"username": created.Username, "role": created.Role}, "")
	s.redirectMasterResult(w, r, "passwords", "利用者を追加しました。", "")
}

func (s *Server) masterPasswordUserChange(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id := r.PathValue("id")
	if err := s.store.ChangeManagedUserPassword(r.Context(), user.OrganizationID, id,
		r.FormValue("password")); err != nil {
		s.redirectMasterResult(w, r, "passwords", "", masterPasswordError(err))
		return
	}
	s.auditTransaction(r, user, "user", id, "user.password_changed", nil, "")
	s.redirectMasterResult(w, r, "passwords", "パスワードを変更しました。", "")
}

func (s *Server) masterPasswordUserDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id := r.PathValue("id")
	if id == user.ID {
		s.redirectMasterResult(w, r, "passwords", "", "ログイン中の利用者は削除できません。")
		return
	}
	if err := s.store.DeleteManagedUser(r.Context(), user.OrganizationID, id); err != nil {
		s.redirectMasterResult(w, r, "passwords", "", masterPasswordError(err))
		return
	}
	s.auditTransaction(r, user, "user", id, "user.deleted", nil, "")
	s.redirectMasterResult(w, r, "passwords", "利用者を削除しました。", "")
}

func (s *Server) masterGuestPasswordChange(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id := r.PathValue("id")
	if err := s.store.ChangeGuestPassword(r.Context(), user.OrganizationID, id, user.ID,
		r.FormValue("password")); err != nil {
		s.redirectMasterResult(w, r, "passwords", "", masterPasswordError(err))
		return
	}
	s.auditTransaction(r, user, "guest_credential", id, "guest.password_changed", nil, "")
	s.redirectMasterResult(w, r, "passwords", "ゲストパスワードを変更しました。", "")
}

func (s *Server) masterGuestPasswordDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id := r.PathValue("id")
	if err := s.store.DeleteGuestCredential(r.Context(), user.OrganizationID, id); err != nil {
		s.redirectMasterResult(w, r, "passwords", "", masterPasswordError(err))
		return
	}
	s.auditTransaction(r, user, "guest_credential", id, "guest.credential_deleted", nil, "")
	s.redirectMasterResult(w, r, "passwords", "ゲスト認証情報を削除しました。", "")
}

func (s *Server) masterGuestPasswordsBulk(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	token, err := database.RandomToken()
	if err != nil {
		s.redirectMasterResult(w, r, "passwords", "", "一時パスワードを生成できませんでした。")
		return
	}
	temporaryPassword := token[:16]
	count, err := s.store.RotateGuestPasswords(r.Context(), user.OrganizationID, user.ID, temporaryPassword)
	if err != nil {
		s.redirectMasterResult(w, r, "passwords", "", masterPasswordError(err))
		return
	}
	s.auditTransaction(r, user, "guest_credential", user.OrganizationID,
		"guest.passwords_rotated", map[string]int64{"count": count}, "")
	s.redirectMasterResult(w, r, "passwords",
		fmt.Sprintf("ゲスト%d件のPWを変更しました。一時PW: %s", count, temporaryPassword), "")
}

func (s *Server) masterPasswordNotify(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	users, err := s.store.Users(r.Context(), user.OrganizationID)
	if err != nil {
		s.redirectMasterResult(w, r, "passwords", "", "通知対象を取得できませんでした。")
		return
	}
	guests, err := s.store.GuestCredentials(r.Context(), user.OrganizationID)
	if err != nil {
		s.redirectMasterResult(w, r, "passwords", "", "通知対象を取得できませんでした。")
		return
	}
	// This application intentionally has no external mail transport in the mock
	// phase. Persisting the accepted notification job in the audit trail makes
	// the operation observable without claiming that an email was delivered.
	count := len(users) + len(guests)
	s.auditTransaction(r, user, "password_notification", user.OrganizationID,
		"password.notification_queued", map[string]int{"recipients": count}, "")
	s.redirectMasterResult(w, r, "passwords",
		fmt.Sprintf("%d件の通知メール送信を受け付けました。", count), "")
}

func masterPasswordError(err error) string {
	switch {
	case errors.Is(err, database.ErrManagedUserExists):
		return "同じログインIDが既に登録されています。"
	case errors.Is(err, database.ErrManagedUserNotFound):
		return "対象の利用者が見つかりません。"
	case errors.Is(err, database.ErrGuestCompanyNotFound):
		return "対象のゲストが見つかりません。"
	default:
		return err.Error()
	}
}
