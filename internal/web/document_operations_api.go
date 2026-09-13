package web

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

type csvExportData struct {
	DocumentType string
	Headers      []string
	Rows         [][]string
}

func (s *Server) apiShipmentTrackingUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "出荷伝票の配送情報変更")
	if !ok {
		return
	}
	var input struct {
		Carrier        string `json:"carrier"`
		TrackingNumber string `json:"trackingNumber"`
		Confirmed      bool   `json:"confirmed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || len(input.Carrier) > 100 || len(input.TrackingNumber) > 200 {
		writeAPIError(w, http.StatusBadRequest, "invalid_tracking", "配送会社と追跡番号を確認してください。")
		return
	}
	record, err := s.repository.UpdateShipmentTracking(r.Context(), user.OrganizationID, r.PathValue("id"), input.Carrier, input.TrackingNumber)
	if err != nil {
		writeShipmentError(w, err)
		return
	}
	after, _ := json.Marshal(map[string]string{"carrier": record.Carrier, "trackingNumber": record.TrackingNumber})
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "shipment_slip", TargetID: record.ID, Action: "shipment.tracking.updated", AfterJSON: string(after),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context())})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiReturnTrackingUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "返品伝票の配送情報変更")
	if !ok {
		return
	}
	var input struct {
		Carrier        string `json:"carrier"`
		TrackingNumber string `json:"trackingNumber"`
		Confirmed      bool   `json:"confirmed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || len(input.Carrier) > 100 || len(input.TrackingNumber) > 200 {
		writeAPIError(w, http.StatusBadRequest, "invalid_tracking", "配送会社と追跡番号を確認してください。")
		return
	}
	record, err := s.repository.UpdateReturnTracking(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, input.Carrier, input.TrackingNumber, input.Confirmed)
	if err != nil {
		if errors.Is(err, persistence.ErrReturnState) {
			writeAPIError(w, http.StatusConflict, "invalid_return_state", "配送番号を入力してから確定してください。")
			return
		}
		writeAPIError(w, http.StatusNotFound, "return_not_found", "返品伝票が見つかりません。")
		return
	}
	after, _ := json.Marshal(map[string]any{"carrier": record.Carrier, "trackingNumber": record.TrackingNumber, "confirmed": input.Confirmed})
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "return_slip", TargetID: record.ID, Action: "return.tracking.updated", AfterJSON: string(after),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context())})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiDocumentEvents(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.DocumentEvents(r.Context(), user.OrganizationID,
		r.URL.Query().Get("documentType"), r.URL.Query().Get("documentId"), limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "document_history_unavailable", "帳票履歴を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiDocumentEventCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.DocumentEventInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_document_event", "帳票履歴の入力内容を確認してください。")
		return
	}
	input.StorageDriver = s.objects.Driver()
	user, _ := currentUser(r.Context())
	record, err := s.repository.RecordDocumentEvent(r.Context(), user.OrganizationID, user.ID, input)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_document_event", "帳票履歴を登録できませんでした。")
		return
	}
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "document_generation_event", TargetID: record.ID, Action: "document." + record.Action,
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context())})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiCSVExport(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	kind := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(r.PathValue("kind"))), ".csv")
	data, err := s.csvExportData(r, user.OrganizationID, kind)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "csv_export_failed", "CSVを作成できませんでした。")
		return
	}
	fileName := fmt.Sprintf("zaiko-%s-%s.csv", kind, time.Now().In(time.FixedZone("JST", 9*60*60)).Format("20060102-150405"))
	metadata, _ := json.Marshal(map[string]any{"rows": len(data.Rows), "kind": kind})
	event, err := s.repository.RecordDocumentEvent(r.Context(), user.OrganizationID, user.ID, persistence.DocumentEventInput{
		DocumentType: data.DocumentType, Action: "download", OutputFormat: "csv", FileName: fileName,
		StorageDriver: s.objects.Driver(), Metadata: metadata,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "document_history_failed", "CSVの発行履歴を保存できませんでした。")
		return
	}
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "document_generation_event", TargetID: event.ID, Action: "csv.exported", Result: "success",
		Reason: fmt.Sprintf("kind=%s rows=%d", kind, len(data.Rows)), IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context())})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write(data.Headers)
	for _, row := range data.Rows {
		for index := range row {
			row[index] = safeCSVCell(row[index])
		}
		_ = writer.Write(row)
	}
	writer.Flush()
}

func (s *Server) csvExportData(r *http.Request, organizationID, kind string) (csvExportData, error) {
	switch kind {
	case "inventory", "stocktake":
		var rows [][]string
		for page := 1; ; page++ {
			result, err := s.repository.Products(r.Context(), organizationID, persistence.ProductFilter{Page: page, PageSize: 100, IncludeCancelled: true, Sort: "code_asc"})
			if err != nil {
				return csvExportData{}, err
			}
			for _, item := range result.Items {
				rows = append(rows, []string{item.ProductCode, item.SKU, item.Brand, item.ModelNumber, item.ReferenceNumber,
					item.SerialNumber, string(item.PurchaseDate), item.SupplierName, strconv.FormatInt(item.CostAmountMinor, 10),
					item.CostCurrency, strconv.FormatInt(item.BaseSalePriceMinor, 10), item.BaseSaleCurrency, item.InventoryStatus})
			}
			if page >= result.TotalPages {
				break
			}
		}
		return csvExportData{DocumentType: kind, Headers: []string{"管理番号", "SKU", "ブランド", "モデル", "型番", "シリアル", "仕入日", "仕入先", "原価", "原価通貨", "売価", "売価通貨", "ステータス"}, Rows: rows}, nil
	case "market":
		items, err := s.repository.MarketPrices(r.Context(), organizationID, 1000)
		if err != nil {
			return csvExportData{}, err
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{string(item.ImportDate), item.BrandCode, item.BrandName, item.ModelNumber,
				item.ReferenceNumber, item.SerialNumber, item.ConditionCode, strconv.FormatInt(item.PurchasePriceMinor, 10),
				item.PurchaseCurrency, strconv.FormatInt(item.MarketPriceMinor, 10), item.MarketCurrency, item.SupplierCode, item.Source})
		}
		return csvExportData{DocumentType: "market", Headers: []string{"取込日", "ブランドコード", "ブランド", "モデル", "型番", "シリアル", "コンディションコード", "原価", "原価通貨", "相場価格", "相場通貨", "仕入先コード", "取込元"}, Rows: rows}, nil
	case "purchases":
		items, err := s.repository.PurchaseSlips(r.Context(), organizationID, 500)
		if err != nil {
			return csvExportData{}, err
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			detail, detailErr := s.repository.PurchaseSlip(r.Context(), organizationID, item.ID)
			if detailErr != nil {
				return csvExportData{}, detailErr
			}
			var units int
			var totalJPY, totalUSD int64
			for _, line := range detail.Lines {
				units += line.Quantity
				if line.CostCurrency == "USD" {
					totalUSD += line.UnitCostMinor * int64(line.Quantity)
				} else {
					totalJPY += line.UnitCostMinor * int64(line.Quantity)
				}
			}
			rows = append(rows, []string{item.SlipNumber, string(item.PurchaseDate), item.SupplierCode, item.SupplierName, item.StaffCode,
				strconv.Itoa(units), strconv.FormatInt(totalJPY, 10), strconv.FormatInt(totalUSD, 10), item.Status, item.Notes})
		}
		return csvExportData{DocumentType: "purchase", Headers: []string{"仕入伝票番号", "仕入日", "仕入先コード", "仕入先", "バイヤーコード", "点数", "合計JPY", "合計USD", "ステータス", "備考"}, Rows: rows}, nil
	case "sales":
		items, err := s.repository.SaleSlips(r.Context(), organizationID, 500)
		if err != nil {
			return csvExportData{}, err
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{item.SlipNumber, string(item.SaleDate), item.BuyerCode, item.BuyerName, item.DisplayCurrency,
				strconv.FormatInt(item.SubtotalMinor, 10), strconv.FormatInt(item.TaxAmountMinor, 10), strconv.FormatInt(item.TotalMinor, 10),
				strconv.FormatInt(item.ConvertedTotalJPY, 10), item.TaxMode, item.Status, item.Notes})
		}
		return csvExportData{DocumentType: "sale", Headers: []string{"売上伝票番号", "売上日", "販売先コード", "販売先", "通貨", "小計", "税額", "合計", "円換算合計", "税区分", "状態", "備考"}, Rows: rows}, nil
	case "shipments":
		items, err := s.repository.ShipmentSlips(r.Context(), organizationID, 500)
		if err != nil {
			return csvExportData{}, err
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{item.SlipNumber, string(item.ShipmentDate), item.BuyerCode, item.BuyerName, item.RecipientName,
				item.RecipientAddress, item.Carrier, item.TrackingNumber, item.SalesSlipNumber, item.Status, item.Notes})
		}
		return csvExportData{DocumentType: "shipment", Headers: []string{"出荷伝票番号", "出荷日", "販売先コード", "販売先", "受取人", "配送先", "配送会社", "追跡番号", "売上伝票番号", "状態", "備考"}, Rows: rows}, nil
	case "returns":
		items, err := s.repository.ReturnSlips(r.Context(), organizationID, 500)
		if err != nil {
			return csvExportData{}, err
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{item.SlipNumber, string(item.TransactionDate), item.OperationType, item.BuyerCode, item.SupplierCode,
				item.PurchaseSlipNo, item.Carrier, item.TrackingNumber, item.Status, item.Reason, item.Notes})
		}
		return csvExportData{DocumentType: "return", Headers: []string{"返品伝票番号", "処理日", "区分", "販売先コード", "仕入先コード", "元仕入伝票", "配送会社", "追跡番号", "状態", "理由", "備考"}, Rows: rows}, nil
	case "documents":
		items, err := s.repository.Documents(r.Context(), organizationID, 1000)
		if err != nil {
			return csvExportData{}, err
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{item.DocumentType, item.Number, string(item.Date), item.PartnerCode, item.PartnerName,
				strconv.FormatInt(item.TotalJPY, 10), strconv.FormatInt(item.TotalUSD, 10), item.Status, item.UpdatedAt.Format(time.RFC3339)})
		}
		return csvExportData{DocumentType: "documents", Headers: []string{"伝票種別", "伝票番号", "日付", "取引先コード", "取引先", "合計JPY", "合計USD", "状態", "更新日時"}, Rows: rows}, nil
	default:
		return csvExportData{}, fmt.Errorf("unsupported csv export kind %q", kind)
	}
}
