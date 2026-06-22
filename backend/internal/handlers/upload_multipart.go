package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authpkg "videohub/internal/auth"
	"videohub/internal/models"
)

// Порог, выше которого фронт обязан использовать multipart (см. ТЗ: > 100 МБ).
const multipartThreshold = 100 * 1024 * 1024

// Размер части по умолчанию для multipart-загрузки.
const defaultPartSize = 50 * 1024 * 1024

// initUploadReq — общее тело для upload-url и multipart/init.
type initUploadReq struct {
	FileName    string `json:"fileName"`
	SizeBytes   int64  `json:"sizeBytes"`
	ContentType string `json:"contentType"`
	PartSize    int64  `json:"partSize,omitempty"`
}

// uploadDescriptor — инструкция клиенту: куда и сколькими частями лить файл.
type uploadDescriptor struct {
	UploadID        string `json:"uploadId"`
	Multipart       bool   `json:"multipart"`
	TotalParts      int    `json:"totalParts"`
	PartSize        int64  `json:"partSize"`
	PartURLTemplate string `json:"partUrlTemplate"` // .../multipart/{id}/part/{partNumber}
	CompleteURL     string `json:"completeUrl"`
}

// POST /api/videos/upload-url
//
// Локальный аналог presigned S3 URL: для файлов <= 100 МБ. Возвращает сессию и
// URL для заливки одним PUT-запросом (totalParts = 1). Для больших файлов клиент
// должен вызвать /multipart/init.
func (h *Handlers) UploadURL(w http.ResponseWriter, r *http.Request) {
	if authpkg.Role(r) == "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admins can upload ad files only")
		return
	}
	var req initUploadReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	ext, ok := validVideoFileName(req.FileName)
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "validation", "file must be .mp4 or .webm")
		return
	}
	if req.SizeBytes > h.Cfg.MaxVideoBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "validation", "file too large")
		return
	}
	if req.SizeBytes > multipartThreshold {
		writeError(w, http.StatusBadRequest, "use_multipart",
			"file larger than 100MB must be uploaded via /api/videos/multipart/init")
		return
	}

	sess, err := h.Store.CreateUploadSession(r.Context(), models.UploadSession{
		UserID:      authpkg.UserID(r),
		FileName:    req.FileName,
		Ext:         ext,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		TotalParts:  1,
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, h.descriptor(sess))
}

// POST /api/videos/multipart/init
func (h *Handlers) MultipartInit(w http.ResponseWriter, r *http.Request) {
	if authpkg.Role(r) == "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admins can upload ad files only")
		return
	}
	var req initUploadReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	ext, ok := validVideoFileName(req.FileName)
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "validation", "file must be .mp4 or .webm")
		return
	}
	if req.SizeBytes <= 0 {
		writeError(w, http.StatusBadRequest, "validation", "sizeBytes is required for multipart upload")
		return
	}
	if req.SizeBytes > h.Cfg.MaxVideoBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "validation", "file too large")
		return
	}

	partSize := req.PartSize
	if partSize <= 0 {
		partSize = defaultPartSize
	}
	totalParts := int((req.SizeBytes + partSize - 1) / partSize)
	if totalParts < 1 {
		totalParts = 1
	}

	sess, err := h.Store.CreateUploadSession(r.Context(), models.UploadSession{
		UserID:      authpkg.UserID(r),
		FileName:    req.FileName,
		Ext:         ext,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		TotalParts:  totalParts,
	})
	if handleStoreErr(w, err) {
		return
	}
	d := h.descriptor(sess)
	d.PartSize = partSize
	writeJSON(w, http.StatusCreated, d)
}

// PUT /api/videos/multipart/{uploadId}/part/{partNumber}
//
// Тело запроса — сырые байты части (без multipart/form-data). partNumber 1-based.
func (h *Handlers) MultipartPart(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	partNumber, err := strconv.Atoi(chi.URLParam(r, "partNumber"))
	if err != nil || partNumber < 1 {
		writeError(w, http.StatusBadRequest, "validation", "partNumber must be a positive integer")
		return
	}

	sess, err := h.Store.GetUploadSession(r.Context(), uploadID)
	if handleStoreErr(w, err) {
		return
	}
	if sess.UserID != authpkg.UserID(r) {
		writeError(w, http.StatusForbidden, "forbidden", "not your upload session")
		return
	}
	if sess.Status != models.UploadOpen {
		writeError(w, http.StatusConflict, "session_closed", "upload session is not open")
		return
	}
	if partNumber > sess.TotalParts {
		writeError(w, http.StatusBadRequest, "validation", "partNumber exceeds totalParts")
		return
	}

	// Ограничиваем суммарный приём, чтобы не переполнить диск сверх лимита.
	remaining := h.Cfg.MaxVideoBytes - sess.ReceivedBytes
	if remaining < 0 {
		remaining = 0
	}
	r.Body = http.MaxBytesReader(w, r.Body, remaining+1)

	partDir, err := h.uploadTmpDir(uploadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload", err.Error())
		return
	}
	partPath := filepath.Join(partDir, strconv.Itoa(partNumber)+".part")

	dst, err := os.Create(partPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload", err.Error())
		return
	}
	n, copyErr := io.Copy(dst, r.Body)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(partPath)
		writeError(w, http.StatusBadRequest, "upload", "failed to read part body: "+copyErr.Error())
		return
	}
	if closeErr != nil {
		_ = os.Remove(partPath)
		writeError(w, http.StatusInternalServerError, "upload", closeErr.Error())
		return
	}

	updated, err := h.Store.AddUploadSessionPart(r.Context(), uploadID, n)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"uploadId":      uploadID,
		"partNumber":    partNumber,
		"bytesReceived": n,
		"receivedParts": updated.ReceivedParts,
		"totalParts":    updated.TotalParts,
	})
}

// POST /api/videos/multipart/complete  { "uploadId": "..." }
//
// Склеивает части в итоговый файл uploads/MP4/<uuid>.<ext> и возвращает публичный URL.
// Большие файлы кладутся только на диск (ServeUpload отдаёт их с fallback на диск,
// Range поддерживается) — в БД bytea их не дублируем.
func (h *Handlers) MultipartComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UploadID string `json:"uploadId"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	sess, err := h.Store.GetUploadSession(r.Context(), req.UploadID)
	if handleStoreErr(w, err) {
		return
	}
	if sess.UserID != authpkg.UserID(r) {
		writeError(w, http.StatusForbidden, "forbidden", "not your upload session")
		return
	}
	if sess.Status == models.UploadCompleted && sess.FinalURL != "" {
		writeJSON(w, http.StatusOK, map[string]any{"url": sess.FinalURL, "uploadId": sess.ID})
		return
	}
	if sess.ReceivedParts < sess.TotalParts {
		writeError(w, http.StatusConflict, "incomplete",
			"received "+strconv.Itoa(sess.ReceivedParts)+" of "+strconv.Itoa(sess.TotalParts)+" parts")
		return
	}

	fileName := uuid.NewString() + sess.Ext
	relPath := "MP4/" + fileName
	finalAbs, err := filepath.Abs(filepath.Join(h.Cfg.UploadsDir, "MP4", fileName))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload", err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(finalAbs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "upload", err.Error())
		return
	}

	partDir, err := h.uploadTmpDir(req.UploadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload", err.Error())
		return
	}
	if err := assembleParts(partDir, sess.TotalParts, finalAbs); err != nil {
		_ = os.Remove(finalAbs)
		writeError(w, http.StatusInternalServerError, "upload", "failed to assemble parts: "+err.Error())
		return
	}
	_ = os.RemoveAll(partDir) // части больше не нужны

	finalURL := strings.TrimRight(h.Cfg.PublicBaseURL, "/") + "/uploads/" + relPath
	if err := h.Store.CompleteUploadSession(r.Context(), sess.ID, relPath, finalURL); handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": finalURL, "uploadId": sess.ID})
}

// --- helpers ---

func (h *Handlers) descriptor(sess *models.UploadSession) uploadDescriptor {
	base := strings.TrimRight(h.Cfg.PublicBaseURL, "/")
	return uploadDescriptor{
		UploadID:        sess.ID,
		Multipart:       sess.TotalParts > 1,
		TotalParts:      sess.TotalParts,
		PartSize:        defaultPartSize,
		PartURLTemplate: base + "/api/videos/multipart/" + sess.ID + "/part/{partNumber}",
		CompleteURL:     base + "/api/videos/multipart/complete",
	}
}

func (h *Handlers) uploadTmpDir(uploadID string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(h.Cfg.UploadsDir, "TMP", uploadID))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// assembleParts последовательно дописывает части 1..total в dst.
func assembleParts(partDir string, total int, dstPath string) error {
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	for i := 1; i <= total; i++ {
		partPath := filepath.Join(partDir, strconv.Itoa(i)+".part")
		src, err := os.Open(partPath)
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	return dst.Sync()
}

func validVideoFileName(name string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	if !allowedVideoExt[ext] {
		return "", false
	}
	return ext, true
}
