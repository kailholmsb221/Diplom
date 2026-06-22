package handlers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authpkg "videohub/internal/auth"
	"videohub/internal/models"
	"videohub/internal/store"
)

var (
	allowedVideoExt = map[string]bool{".mp4": true, ".webm": true}
	allowedImageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}
	allowedAudioExt = map[string]bool{
		".mp3": true, ".wav": true, ".m4a": true, ".aac": true,
		".ogg": true, ".opus": true, ".flac": true,
	}
)

func (h *Handlers) UploadVideo(w http.ResponseWriter, r *http.Request) {
	if authpkg.Role(r) == "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admins can upload ad files only")
		return
	}

	url, err := h.saveUpload(r, "MP4", h.Cfg.MaxVideoBytes, allowedVideoExt)
	if err != nil {
		writeUploadErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}

func (h *Handlers) UploadAd(w http.ResponseWriter, r *http.Request) {
	url, err := h.saveUpload(r, "ADS", h.Cfg.MaxVideoBytes, allowedVideoExt)
	if err != nil {
		writeUploadErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}

// UploadAudio принимает аудиофайл (музыка / озвучка) для аудиодорожки редактора.
// Кладёт его в uploads/AUDIO и регистрирует завершённую upload-сессию, чтобы файл
// стал легальным источником аудиоклипа при экспорте (см. AuthorizeClipSource).
func (h *Handlers) UploadAudio(w http.ResponseWriter, r *http.Request) {
	if authpkg.Role(r) == "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admins can upload ad files only")
		return
	}

	url, err := h.saveUpload(r, "AUDIO", h.Cfg.MaxVideoBytes, allowedAudioExt)
	if err != nil {
		writeUploadErr(w, err)
		return
	}

	// Привязываем файл к пользователю как завершённую сессию — иначе экспорт
	// отклонит источник (нет прав использовать). Не критично для самой загрузки.
	if sess, cerr := h.Store.CreateUploadSession(r.Context(), models.UploadSession{
		UserID:     authpkg.UserID(r),
		FileName:   "audio",
		TotalParts: 1,
	}); cerr == nil {
		_ = h.Store.CompleteUploadSession(r.Context(), sess.ID, "", url)
	}

	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}

func (h *Handlers) UploadImage(w http.ResponseWriter, r *http.Request) {
	url, err := h.saveUpload(r, "PNG", h.Cfg.MaxImageBytes, allowedImageExt)
	if err != nil {
		writeUploadErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}

func (h *Handlers) ServeUpload(w http.ResponseWriter, r *http.Request) {
	subdir := chi.URLParam(r, "subdir")
	fileName := chi.URLParam(r, "file")
	uploadPath := subdir + "/" + fileName

	file, err := h.Store.GetUploadFile(r.Context(), uploadPath)
	if err == nil {
		w.Header().Set("Content-Type", file.ContentType)
		http.ServeContent(w, r, fileName, file.CreatedAt, bytes.NewReader(file.Data))
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "upload", err.Error())
		return
	}

	http.ServeFile(w, r, filepath.Join(h.Cfg.UploadsDir, uploadPath))
}

func (h *Handlers) saveUpload(r *http.Request, subdir string, maxBytes int64, allowed map[string]bool) (string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes+1024)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return "", uploadError{status: http.StatusBadRequest, msg: "invalid multipart: " + err.Error()}
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		return "", uploadError{status: http.StatusBadRequest, msg: "form field 'file' is required"}
	}
	defer file.Close()

	if header.Size > maxBytes {
		return "", uploadError{status: http.StatusRequestEntityTooLarge, msg: "file too large"}
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowed[ext] {
		return "", uploadError{status: http.StatusUnsupportedMediaType, msg: "unsupported file extension: " + ext}
	}

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxBytes {
		return "", uploadError{status: http.StatusRequestEntityTooLarge, msg: "file too large"}
	}

	fileName := uuid.NewString() + ext
	uploadPath := subdir + "/" + fileName
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	if err := h.Store.SaveUploadFile(r.Context(), store.UploadFile{
		Path:        uploadPath,
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		Data:        data,
	}); err != nil {
		return "", err
	}

	if err := h.cacheUploadOnDisk(subdir, fileName, data); err != nil {
		return strings.TrimRight(h.Cfg.PublicBaseURL, "/") + "/uploads/" + uploadPath, nil
	}

	return strings.TrimRight(h.Cfg.PublicBaseURL, "/") + "/uploads/" + uploadPath, nil
}

func (h *Handlers) cacheUploadOnDisk(subdir, fileName string, data []byte) error {
	dirAbs, err := filepath.Abs(filepath.Join(h.Cfg.UploadsDir, subdir))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirAbs, 0o755); err != nil {
		return err
	}

	full := filepath.Join(dirAbs, fileName)
	dst, err := os.Create(full)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := dst.Write(data); err != nil {
		_ = os.Remove(full)
		return err
	}
	return nil
}

type uploadError struct {
	status int
	msg    string
}

func (e uploadError) Error() string { return e.msg }

func writeUploadErr(w http.ResponseWriter, err error) {
	var ue uploadError
	if errors.As(err, &ue) {
		writeError(w, ue.status, "upload", ue.msg)
		return
	}
	writeError(w, http.StatusInternalServerError, "upload", err.Error())
}
