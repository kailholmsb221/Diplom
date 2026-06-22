package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	authpkg "videohub/internal/auth"
	"videohub/internal/models"
	"videohub/internal/recommend"
	"videohub/internal/store"
)

// GET /api/videos?q=&channelId=&limit=&category=
func (h *Handlers) ListVideos(w http.ResponseWriter, r *http.Request) {
	params := store.ListVideosParams{
		Query:      r.URL.Query().Get("q"),
		ChannelID:  r.URL.Query().Get("channelId"),
		Category:   r.URL.Query().Get("category"),
		OnlyPublic: r.URL.Query().Get("onlyPublic") == "true",
	}
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		params.Limit = l
	}
	videos, err := h.Store.ListVideos(r.Context(), params)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, videos)
}

// GET /api/videos/{id}
func (h *Handlers) GetVideo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, err := h.Store.GetVideo(r.Context(), id)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// DELETE /api/videos/{id}
func (h *Handlers) DeleteVideo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteVideo(r.Context(), id); handleStoreErr(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createVideoReq — JSON-форма создания (для случая, когда файлы уже загружены).
type createVideoReq struct {
	ChannelID    string   `json:"channelId"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	ThumbnailURL string   `json:"thumbnailUrl"`
	VideoURL     string   `json:"videoUrl"`
	DurationSec  int      `json:"durationSec"`
	Category     string   `json:"category"`
	Visibility   string   `json:"visibility"`
	Tags         []string `json:"tags"`
}

// POST /api/videos — создаёт видео (ссылки на файлы предполагаются уже загруженными).
func (h *Handlers) CreateVideo(w http.ResponseWriter, r *http.Request) {
	if authpkg.Role(r) == "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admins can create ads only, not channel videos")
		return
	}

	var req createVideoReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid json: "+err.Error())
		return
	}
	if req.Title == "" || req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "validation", "title and channelId are required")
		return
	}
	req.VideoURL = strings.TrimSpace(req.VideoURL)
	if !isUploadedChannelVideoURL(req.VideoURL) {
		writeError(w, http.StatusBadRequest, "validation", "video must be uploaded as a file; YouTube or external video links are not allowed")
		return
	}
	if req.Visibility == "" {
		req.Visibility = "public"
	}
	if req.Category == "" {
		req.Category = "other"
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	req.Tags = normalizeTags(req.Tags)
	v, err := h.Store.CreateVideo(r.Context(), models.Video{
		ChannelID:    req.ChannelID,
		Title:        req.Title,
		Description:  req.Description,
		ThumbnailURL: req.ThumbnailURL,
		VideoURL:     req.VideoURL,
		DurationSec:  req.DurationSec,
		Category:     req.Category,
		Visibility:   req.Visibility,
		Tags:         req.Tags,
	})
	if handleStoreErr(w, err) {
		return
	}

	// Если видео физически лежит в uploads — запускаем транскрипцию в фоне.
	// Внешние URL (например googleapis) пропускаются.
	if localPath, ok := h.localFileFromURL(req.VideoURL); ok {
		if err := h.Store.SetVideoTranscriptStatus(r.Context(), v.ID, models.TranscriptProcessing, ""); err == nil {
			h.Transcriber.Transcribe(v.ID, localPath)
			v.TranscriptStatus = models.TranscriptProcessing
		}
	}

	writeJSON(w, http.StatusCreated, v)
}

// POST /api/videos/{id}/views — засчитывает уникальный просмотр.
// Если пользователь уже смотрел это видео — счётчик не увеличивается.
// Для анонимных запросов просмотр не засчитывается вовсе
// (берём id строго из JWT, без fallback на DEFAULT_USER_ID).
func (h *Handlers) RegisterView(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	uid := authpkg.UserID(r)
	counted, err := h.Store.RegisterView(r.Context(), id, uid)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"counted": counted})
}

// POST /api/videos/{id}/reaction { "reaction": "like" | "dislike" | "" }
func (h *Handlers) SetReaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Reaction string `json:"reaction"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.Reaction != "" && req.Reaction != "like" && req.Reaction != "dislike" {
		writeError(w, http.StatusBadRequest, "validation", "reaction must be 'like', 'dislike' or empty")
		return
	}
	prev, err := h.Store.SetReaction(r.Context(), id, h.currentUserID(r), req.Reaction)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"previous": prev, "current": fallback(req.Reaction, "none")})
}

// GET /api/videos/{id}/reaction — текущая реакция пользователя.
func (h *Handlers) GetReaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cur, err := h.Store.GetReaction(r.Context(), id, h.currentUserID(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"reaction": fallback(cur, "")})
}

// GET /api/videos/recommended
func (h *Handlers) Recommended(w http.ResponseWriter, r *http.Request) {
	limit := 24
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	videos, err := h.Store.ListVideos(r.Context(), store.ListVideosParams{OnlyPublic: true, Limit: 100, OrderBy: "rating"})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, recommend.Rank(videos, "", limit))
}

// GET /api/videos/{id}/recommendations — рекомендации рядом с плеером.
func (h *Handlers) Recommendations(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	current, err := h.Store.GetVideo(r.Context(), id)
	if handleStoreErr(w, err) {
		return
	}
	pool, err := h.Store.ListVideos(r.Context(), store.ListVideosParams{
		OnlyPublic: true,
		ExcludeID:  id,
		Limit:      200,
		OrderBy:    "rating",
	})
	if handleStoreErr(w, err) {
		return
	}
	limit := 12
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	writeJSON(w, http.StatusOK, recommend.Rank(pool, current.Category, limit))
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimLeft(tag, "#"))
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func isUploadedChannelVideoURL(videoURL string) bool {
	const marker = "/uploads/MP4/"
	idx := strings.Index(videoURL, marker)
	if idx < 0 {
		return false
	}
	fileName := videoURL[idx+len(marker):]
	if fileName == "" || strings.Contains(fileName, "/") || strings.Contains(fileName, "\\") {
		return false
	}
	lower := strings.ToLower(fileName)
	return strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".webm")
}
