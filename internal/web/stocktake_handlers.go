package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) stocktakes(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	list, err := s.store.Stocktakes(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "棚卸情報を取得できませんでした", http.StatusInternalServerError)
		return
	}
	var current database.Stocktake
	for _, item := range list {
		if item.Status == "draft" {
			current, err = s.store.Stocktake(r.Context(), user.OrganizationID, item.ID)
			if err != nil {
				http.Error(w, "棚卸明細を取得できませんでした", http.StatusInternalServerError)
				return
			}
			break
		}
	}
	if current.ID == "" {
		today := time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
		current, err = s.store.CreateStocktake(r.Context(), user.OrganizationID, today, "", user.ID)
		if err != nil && !errors.Is(err, database.ErrStocktakeInProgress) {
			http.Error(w, "棚卸を初期化できませんでした", http.StatusInternalServerError)
			return
		}
		if errors.Is(err, database.ErrStocktakeInProgress) {
			list, _ = s.store.Stocktakes(r.Context(), user.OrganizationID)
			for _, item := range list {
				if item.Status == "draft" {
					current, err = s.store.Stocktake(r.Context(), user.OrganizationID, item.ID)
					break
				}
			}
		} else {
			list, _ = s.store.Stocktakes(r.Context(), user.OrganizationID)
			after, _ := json.Marshal(current)
			_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
				OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "stocktake",
				TargetID: current.ID, Action: "stocktake.auto_initialized", AfterJSON: string(after), Result: "success",
				IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
			})
		}
		if err != nil {
			http.Error(w, "棚卸を初期化できませんでした", http.StatusInternalServerError)
			return
		}
	}
	s.renderStocktakePage(w, r, user, list, current)
}

func (s *Server) stocktakeDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	current, err := s.store.Stocktake(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, database.ErrStocktakeNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "棚卸明細を取得できませんでした", http.StatusInternalServerError)
		return
	}
	list, err := s.store.Stocktakes(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "棚卸履歴を取得できませんでした", http.StatusInternalServerError)
		return
	}
	s.renderStocktakePage(w, r, user, list, current)
}

func (s *Server) renderStocktakePage(w http.ResponseWriter, r *http.Request, user database.User, list []database.Stocktake, current database.Stocktake) {
	alertCount, err := s.pendingAlerts(r, user.OrganizationID)
	if err != nil {
		http.Error(w, "通知件数を取得できませんでした", http.StatusInternalServerError)
		return
	}
	var inStockCount, shippedCount, inStockCountedCount, shippedCountedCount, pendingCount int
	var inStockTotal, shippedTotal, inStockCountedTotal, shippedCountedTotal, countedTotal, differenceTotal int64
	for _, line := range current.Lines {
		shipped := line.InventoryStatus == "sold" || line.InventoryStatus == "shipped"
		if shipped {
			shippedCount++
			shippedTotal += line.CostAmountMinor
		} else {
			inStockCount++
			inStockTotal += line.CostAmountMinor
		}
		if line.CountedPresent != nil {
			countedTotal += line.CostAmountMinor
			if shipped {
				shippedCountedCount++
				shippedCountedTotal += line.CostAmountMinor
			} else {
				inStockCountedCount++
				inStockCountedTotal += line.CostAmountMinor
			}
			if !*line.CountedPresent {
				differenceTotal += line.CostAmountMinor
			}
		}
		if line.ReviewStatus == "pending" {
			pendingCount++
		}
	}
	s.render(w, "stocktakes", http.StatusOK, pageData{
		Title: "棚卸", Active: "stocktakes", User: user, CSRF: csrfFromRequest(r),
		Stocktakes: list, Stocktake: current,
		StocktakeCanComplete:  current.ID != "" && current.Status == "draft" && current.CountedCount == current.ExpectedCount && pendingCount == 0,
		StocktakeInStockCount: inStockCount, StocktakeShippedCount: shippedCount,
		StocktakeInStockCountedCount: inStockCountedCount, StocktakeShippedCountedCount: shippedCountedCount,
		StocktakeInStockTotal: inStockTotal, StocktakeShippedTotal: shippedTotal,
		StocktakeInStockCountedTotal: inStockCountedTotal, StocktakeShippedCountedTotal: shippedCountedTotal,
		StocktakeCountedTotal: countedTotal, StocktakeDifferenceTotal: differenceTotal,
		StocktakePendingCount: pendingCount,
		TodayISO:              time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02"),
		AlertCount:            alertCount,
		Notice:                r.URL.Query().Get("notice"),
		Error:                 r.URL.Query().Get("error"),
	})
}

func (s *Server) stocktakeCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	stocktake, err := s.store.CreateStocktake(
		r.Context(), user.OrganizationID, r.FormValue("stocktake_date"), r.FormValue("notes"), user.ID,
	)
	if errors.Is(err, database.ErrStocktakeInProgress) {
		list, listErr := s.store.Stocktakes(r.Context(), user.OrganizationID)
		if listErr == nil {
			for _, item := range list {
				if item.Status == "draft" {
					http.Redirect(w, r, "/stocktakes/"+item.ID+"?notice="+url.QueryEscape("実施中の棚卸を表示しました。"), http.StatusSeeOther)
					return
				}
			}
		}
	}
	if err != nil {
		http.Redirect(w, r, "/stocktakes?error="+url.QueryEscape(stocktakeErrorMessage(err)), http.StatusSeeOther)
		return
	}
	after, _ := json.Marshal(stocktake)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "stocktake",
		TargetID: stocktake.ID, Action: "stocktake.create", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/stocktakes/"+stocktake.ID+"?notice="+url.QueryEscape("棚卸を開始しました。"), http.StatusSeeOther)
}

func (s *Server) stocktakeLineUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	stocktakeID := r.PathValue("id")
	result := r.FormValue("result")
	if result != "present" && result != "absent" {
		http.Redirect(w, r, "/stocktakes/"+stocktakeID+"?error="+url.QueryEscape("実地結果を選択してください。"), http.StatusSeeOther)
		return
	}
	var err error
	if result == "present" {
		err = s.store.MarkStocktakePresent(r.Context(), user.OrganizationID, stocktakeID, r.PathValue("lineID"), user.ID)
	} else if user.Role != database.RoleAdmin {
		var approval database.ApprovalRequest
		approval, err = s.store.RequestStocktakeDifferenceApproval(
			r.Context(), user.OrganizationID, stocktakeID, r.PathValue("lineID"),
			r.FormValue("difference_reason"), r.FormValue("notes"), user.ID,
		)
		if err == nil {
			s.auditTransaction(r, user, "approval_request", approval.ID, "approval.requested", approval, approval.RequestReason)
		}
	} else {
		err = s.store.RecordStocktakeDifference(
			r.Context(), user.OrganizationID, stocktakeID, r.PathValue("lineID"),
			r.FormValue("difference_reason"), r.FormValue("notes"), user.ID, false,
		)
	}
	if err != nil {
		http.Redirect(w, r, "/stocktakes/"+stocktakeID+"?error="+url.QueryEscape(stocktakeErrorMessage(err)), http.StatusSeeOther)
		return
	}
	after, _ := json.Marshal(map[string]string{"result": result, "notes": r.FormValue("notes")})
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "stocktake_line",
		TargetID: r.PathValue("lineID"), Action: "stocktake.count", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	notice := "実地結果を保存しました。"
	if result == "absent" && user.Role != database.RoleAdmin {
		notice = "不一致を承認申請しました。"
	}
	http.Redirect(w, r, "/stocktakes/"+stocktakeID+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func (s *Server) stocktakeSave(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	stocktakeID := r.PathValue("id")
	if err := s.store.SaveStocktake(r.Context(), user.OrganizationID, stocktakeID); err != nil {
		http.Redirect(w, r, "/stocktakes/"+stocktakeID+"?error="+url.QueryEscape(stocktakeErrorMessage(err)), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/stocktakes/"+stocktakeID+"?notice="+url.QueryEscape("棚卸内容を一時保存しました。"), http.StatusSeeOther)
}

func (s *Server) stocktakeComplete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	stocktakeID := r.PathValue("id")
	stocktake, err := s.store.CompleteStocktake(r.Context(), user.OrganizationID, stocktakeID, user.ID)
	if err != nil {
		http.Redirect(w, r, "/stocktakes/"+stocktakeID+"?error="+url.QueryEscape(stocktakeErrorMessage(err)), http.StatusSeeOther)
		return
	}
	after, _ := json.Marshal(stocktake)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "stocktake",
		TargetID: stocktake.ID, Action: "stocktake.complete", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/stocktakes/"+stocktakeID+"?notice="+url.QueryEscape("棚卸を確定しました。"), http.StatusSeeOther)
}

func stocktakeErrorMessage(err error) string {
	switch {
	case errors.Is(err, database.ErrStocktakeInProgress):
		return "実施中の棚卸があります。"
	case errors.Is(err, database.ErrStocktakeNotFound):
		return "棚卸が見つかりません。"
	case errors.Is(err, database.ErrStocktakeCompleted):
		return "確定済みの棚卸は変更できません。"
	case errors.Is(err, database.ErrStocktakeUnchecked):
		return "未確認の商品があります。全件確認後に確定してください。"
	case errors.Is(err, database.ErrStocktakePending):
		return "承認待ちの商品があります。承認後に確定してください。"
	case errors.Is(err, database.ErrStocktakeMismatch):
		return "棚卸の件数または金額がシステム集計と一致しません。"
	case errors.Is(err, database.ErrStocktakeDuplicate):
		return "この商品はすでに確認済みです。"
	default:
		return err.Error()
	}
}
