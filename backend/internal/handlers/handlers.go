// Package handlers содержит HTTP-обработчики API.
package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	authpkg "videohub/internal/auth"
	"videohub/internal/config"
	"videohub/internal/render"
	"videohub/internal/store"
	"videohub/internal/transcribe"
)

// Handlers — корневой контейнер для зависимостей хендлеров.
type Handlers struct {
	Store       *store.Store
	Cfg         config.Config
	Transcriber *transcribe.Spawner
	Renderer    *render.Spawner
}

func New(s *store.Store, cfg config.Config) *Handlers {
	return &Handlers{
		Store:       s,
		Cfg:         cfg,
		Transcriber: transcribe.New(cfg, s),
		Renderer:    render.New(cfg, s),
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func handleStoreErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return true
	}
	log.Printf("store error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
	return true
}

// currentUserID возвращает id залогиненного пользователя.
// Приоритет: JWT (из контекста) → ?userId=... → DEFAULT_USER_ID.
// Последние два варианта используются для публичных эндпоинтов и обратной
// совместимости с режимом без авторизации.
func (h *Handlers) currentUserID(r *http.Request) string {
	if id := authpkg.UserID(r); id != "" {
		return id
	}
	if id := r.URL.Query().Get("userId"); id != "" {
		return id
	}
	return h.Cfg.DefaultUserID
}
