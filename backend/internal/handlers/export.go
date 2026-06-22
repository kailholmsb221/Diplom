package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	authpkg "videohub/internal/auth"
	"videohub/internal/models"
)

// POST /api/videos/{id}/export
//
// Тело запроса — EditManifest. Создаёт задачу рендера и запускает worker.
// Прогресс читается через GET .../export/{jobId}/progress (SSE).
func (h *Handlers) ExportVideo(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "id")
	video, err := h.Store.GetVideo(r.Context(), videoID)
	if handleStoreErr(w, err) {
		return
	}

	var m models.EditManifest
	if err := readJSON(r, &m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if msg, ok := h.validateManifest(m); !ok {
		writeError(w, http.StatusBadRequest, "validation", msg)
		return
	}

	vid := video.ID
	job, err := h.Store.CreateRenderJob(r.Context(), &vid, authpkg.UserID(r), m)
	if handleStoreErr(w, err) {
		return
	}

	h.Renderer.Render(job.ID)
	writeJSON(w, http.StatusAccepted, job)
}

// GET /api/videos/{id}/export/{jobId} — текущее состояние задачи (одним JSON).
func (h *Handlers) GetExportJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.Store.GetRenderJob(r.Context(), chi.URLParam(r, "jobId"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// GET /api/videos/{id}/export/{jobId}/progress — поток прогресса через SSE.
//
// SSE выбран вместо WebSocket: в репозитории нет ws-библиотеки, а односторонний
// поток прогресса для этого достаточен. Событие шлётся при каждом изменении
// прогресса (шаг 1%) и финально при completed/failed.
func (h *Handlers) ExportProgress(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(job *models.RenderJob) {
		payload, _ := json.Marshal(map[string]any{
			"jobId":     job.ID,
			"status":    job.Status,
			"progress":  job.Progress,
			"outputUrl": job.OutputURL,
			"error":     job.Error,
		})
		w.Write([]byte("data: "))
		w.Write(payload)
		w.Write([]byte("\n\n"))
		flusher.Flush()
	}

	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()

	ctx := r.Context()
	lastProgress := -1
	lastStatus := ""
	for {
		job, err := h.Store.GetRenderJob(ctx, jobID)
		if err != nil {
			// Контекст отменён клиентом — тихо выходим; иначе шлём ошибку.
			if ctx.Err() == nil {
				w.Write([]byte("event: error\ndata: {\"error\":\"job not found\"}\n\n"))
				flusher.Flush()
			}
			return
		}
		if job.Progress != lastProgress || job.Status != lastStatus {
			send(job)
			lastProgress = job.Progress
			lastStatus = job.Status
		}
		if job.Status == models.RenderCompleted || job.Status == models.RenderFailed {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// validateManifest проверяет, что раскладка пригодна для рендера.
func (h *Handlers) validateManifest(m models.EditManifest) (string, bool) {
	if len(m.Clips) == 0 {
		return "manifest must contain at least one clip", false
	}
	for i, c := range m.Clips {
		if _, ok := h.localFileFromURL(c.SourceURL); !ok {
			return "clip sourceUrl must point to an uploaded file (clip " + strconv.Itoa(i) + ")", false
		}
		if c.TrimStart < 0 || c.TrimEnd <= c.TrimStart {
			return "clip trim range is invalid (clip " + strconv.Itoa(i) + ")", false
		}
	}
	return "", true
}
