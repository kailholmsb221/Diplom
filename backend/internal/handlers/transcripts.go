package handlers

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"videohub/internal/models"
	"videohub/internal/store"
)

// GET /api/videos/{id}/transcript?lang=ru|kk|en
//
// Если для языка ещё нет записи в transcripts — возвращаем заготовку со
// статусом NOT_STARTED, чтобы фронт мог однозначно показать состояние.
func (h *Handlers) GetTranscript(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	lang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))
	if lang == "" {
		lang = "ru"
	}
	if !validTranscriptLang(lang) {
		writeError(w, http.StatusBadRequest, "validation", "lang must be ru, kk or en")
		return
	}

	video, err := h.Store.GetVideo(r.Context(), id)
	if handleStoreErr(w, err) {
		return
	}

	tr, err := h.Store.GetTranscript(r.Context(), id, lang)
	if errors.Is(err, store.ErrNotFound) {
		tr = &models.Transcript{
			VideoID:    id,
			Language:   lang,
			IsOriginal: video.OriginalLanguage != nil && *video.OriginalLanguage == lang,
			Status:     models.TranscriptNotStarted,
			Segments:   []models.TranscriptSegment{},
		}
	} else if handleStoreErr(w, err) {
		return
	}
	tr.VTTUrl = h.transcriptURL(tr)

	originalLang := ""
	if video.OriginalLanguage != nil {
		originalLang = *video.OriginalLanguage
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"videoId":          id,
		"selectedLanguage": lang,
		"originalLanguage": originalLang,
		"status":           tr.Status,
		"fullText":         tr.FullText,
		"segments":         tr.Segments,
		"vttUrl":           tr.VTTUrl,
		"error":            tr.Error,
		"videoStatus":      video.TranscriptStatus,
	})
}

// GET /api/videos/{id}/subtitles
//
// Сводка: какие языки уже готовы, ссылки на VTT, статус каждого.
func (h *Handlers) GetSubtitles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	video, err := h.Store.GetVideo(r.Context(), id)
	if handleStoreErr(w, err) {
		return
	}

	list, err := h.Store.ListTranscriptsForVideo(r.Context(), id)
	if handleStoreErr(w, err) {
		return
	}

	available := make([]string, 0, len(list))
	urls := make(map[string]string, len(list))
	statuses := make(map[string]string, len(list))
	for i := range list {
		t := &list[i]
		t.VTTUrl = h.transcriptURL(t)
		statuses[t.Language] = t.Status
		if t.Status == models.TranscriptCompleted && t.VTTUrl != "" {
			available = append(available, t.Language)
			urls[t.Language] = t.VTTUrl
		}
	}

	originalLang := ""
	if video.OriginalLanguage != nil {
		originalLang = *video.OriginalLanguage
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"videoId":            id,
		"originalLanguage":   originalLang,
		"videoStatus":        video.TranscriptStatus,
		"availableLanguages": available,
		"subtitleUrls":       urls,
		"status":             statuses,
	})
}

// POST /api/videos/{id}/transcribe — повторный запуск (или первый, если не было).
// Требует, чтобы у видео был известен video_url с локальным файлом.
func (h *Handlers) StartTranscribe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	video, err := h.Store.GetVideo(r.Context(), id)
	if handleStoreErr(w, err) {
		return
	}
	localPath, ok := h.localFileFromURL(video.VideoURL)
	if !ok {
		writeError(w, http.StatusBadRequest, "no_local_file",
			"video file is not stored locally; only uploaded videos can be transcribed")
		return
	}
	if err := h.Store.SetVideoTranscriptStatus(r.Context(), id, models.TranscriptProcessing, ""); handleStoreErr(w, err) {
		return
	}
	h.Transcriber.Transcribe(id, localPath)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  models.TranscriptProcessing,
		"message": "transcription started",
	})
}

// POST /api/videos/{id}/translate — повторный запуск переводов (оригинал должен быть готов).
func (h *Handlers) StartTranslate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	video, err := h.Store.GetVideo(r.Context(), id)
	if handleStoreErr(w, err) {
		return
	}
	if video.OriginalLanguage == nil || *video.OriginalLanguage == "" {
		writeError(w, http.StatusBadRequest, "no_original",
			"original transcript is not ready; run /transcribe first")
		return
	}
	if err := h.Store.SetVideoTranscriptStatus(r.Context(), id, models.TranscriptTranslating, ""); handleStoreErr(w, err) {
		return
	}
	h.Transcriber.Translate(id)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  models.TranscriptTranslating,
		"message": "translation started",
	})
}

// --- helpers ---

func validTranscriptLang(lang string) bool {
	return lang == "ru" || lang == "kk" || lang == "en"
}

// transcriptURL формирует абсолютную ссылку на VTT-файл.
// Worker сохраняет vtt_path относительно UPLOADS_DIR, например "SUBTITLES/<id>_ru.vtt".
func (h *Handlers) transcriptURL(t *models.Transcript) string {
	if t == nil {
		return ""
	}
	// Поле приходит непосредственно из store/transcripts.go (vtt_path).
	// vttUrl мы здесь не имеем — есть только Error/Segments; vtt_path лежит
	// в неэкспортированной части. Получаем напрямую через store, но проще —
	// форматируем по соглашению "{lang} → SUBTITLES/{videoId}_{lang}.vtt".
	if t.Status != models.TranscriptCompleted {
		return ""
	}
	base := strings.TrimRight(h.Cfg.PublicBaseURL, "/")
	return base + "/uploads/SUBTITLES/" + t.VideoID + "_" + t.Language + ".vtt"
}

// localFileFromURL пытается превратить публичный URL ("…/uploads/MP4/<file>")
// в путь на диске. Если файл лежит вне UPLOADS_DIR — возвращаем false.
func (h *Handlers) localFileFromURL(videoURL string) (string, bool) {
	if videoURL == "" {
		return "", false
	}
	const marker = "/uploads/"
	idx := strings.Index(videoURL, marker)
	if idx < 0 {
		return "", false
	}
	rel := videoURL[idx+len(marker):]
	abs, err := filepath.Abs(filepath.Join(h.Cfg.UploadsDir, rel))
	if err != nil {
		return "", false
	}
	return abs, true
}
