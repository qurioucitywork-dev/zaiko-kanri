package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiProductFileUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(11 << 20); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_upload", "10MB以下の画像を指定してください。")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "file_required", "画像ファイルを指定してください。")
		return
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil || len(contents) == 0 || len(contents) > 10<<20 {
		writeAPIError(w, http.StatusBadRequest, "invalid_file_size", "画像は1バイト以上10MB以下にしてください。")
		return
	}
	contentType := http.DetectContentType(contents)
	extension := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[contentType]
	if extension == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_file_type", "JPEG、PNG、WebPのいずれかを指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	productID := r.PathValue("id")
	if _, err := s.repository.PrepareProductFile(r.Context(), user.OrganizationID, productID); err != nil {
		status, code, message := http.StatusNotFound, "product_not_found", "商品が見つかりません。"
		if errors.Is(err, persistence.ErrProductFileLimit) {
			status, code, message = http.StatusConflict, "file_limit", "商品画像は最大10枚です。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	fileID, err := database.NewID("fil")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "file_id_failed", "画像を保存できませんでした。")
		return
	}
	objectKey := filepath.ToSlash(filepath.Join("organizations", user.OrganizationID, "products", productID, fileID+extension))
	size, err := s.objects.Put(r.Context(), objectKey, contentType, bytes.NewReader(contents))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "storage_failed", "画像ストレージへ保存できませんでした。")
		return
	}
	digest := sha256.Sum256(contents)
	record, err := s.repository.CreateProductFile(r.Context(), user.OrganizationID, user.ID, persistence.ProductFileRecord{
		ID: fileID, ProductID: productID, StorageDriver: s.objects.Driver(), ObjectKey: objectKey,
		OriginalName: filepath.Base(header.Filename), ContentType: contentType, SizeBytes: size, SHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		_ = s.objects.Delete(r.Context(), objectKey)
		writeAPIError(w, http.StatusInternalServerError, "file_metadata_failed", "画像情報を保存できませんでした。")
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "product_file", TargetID: record.ID, Action: "product_file.uploaded", AfterJSON: string(after),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiProductFile(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.ProductFile(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "file_not_found", "画像が見つかりません。")
		return
	}
	object, err := s.objects.Get(r.Context(), record.ObjectKey)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "file_not_found", "画像が見つかりません。")
		return
	}
	defer object.Body.Close()
	w.Header().Set("Content-Type", record.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(record.SizeBytes, 10))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = io.Copy(w, object.Body)
}

func (s *Server) apiProductFiles(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.repository.ProductFiles(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "files_unavailable", "商品画像を取得できませんでした。")
		return
	}
	type responseFile struct {
		persistence.ProductFileRecord
		URL string `json:"url"`
	}
	items := make([]responseFile, 0, len(records))
	for _, record := range records {
		fileURL := "/api/v1/product-files/" + record.ID
		if record.SHA256 != "" {
			fileURL += "?v=" + record.SHA256
		}
		items = append(items, responseFile{ProductFileRecord: record, URL: fileURL})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) apiProductFileDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.DeleteProductFile(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, persistence.ErrProductUnavailable) {
			writeAPIError(w, http.StatusNotFound, "file_not_found", "画像が見つかりません。")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "file_delete_failed", "画像を削除できませんでした。")
		return
	}
	storageResult := "success"
	if err := s.objects.Delete(r.Context(), record.ObjectKey); err != nil {
		// The metadata has already been removed so the application remains
		// consistent. An orphaned object can be cleaned up operationally.
		storageResult = "metadata_deleted_storage_cleanup_failed"
		s.log.Warn("delete product image object", "error", err, "file_id", record.ID, "request_id", requestID(r.Context()))
	}
	before, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "product_file", TargetID: record.ID, Action: "product_file.deleted", BeforeJSON: string(before),
		Result: storageResult, RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiProductFilesReorder(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FileIDs []string `json:"fileIds"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "画像の並び順を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	productID := r.PathValue("id")
	records, err := s.repository.ReorderProductFiles(r.Context(), user.OrganizationID, productID, input.FileIDs)
	if err != nil {
		if errors.Is(err, persistence.ErrProductFileOrder) {
			writeAPIError(w, http.StatusConflict, "stale_file_order", "画像構成が更新されています。画面を再読み込みしてください。")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "file_order_failed", "画像の並び順を保存できませんでした。")
		return
	}
	after, _ := json.Marshal(records)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "product", TargetID: productID, Action: "product_files.reordered", AfterJSON: string(after),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}
