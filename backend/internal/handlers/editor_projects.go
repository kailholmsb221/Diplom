package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	authpkg "videohub/internal/auth"
	"videohub/internal/models"
)

// ---------------------------------------------------------------------------
// Проекты встроенного видеоредактора.
//
// Монтаж НЕразрушающий: оригинальные файлы не меняются, проект хранит только
// инструкции (timeline). Экспорт рождает НОВОЕ видео на канале пользователя.
// Все операции проверяют владение проектом и легальность источников клипов
// (анти-SSRF + allow_remix) на сервере — одной проверки в UI недостаточно.
// ---------------------------------------------------------------------------

const (
	maxTitleLen = 200
	maxDescLen  = 5000
)

// timeline JSON, который хранит фронт. Сервер парсит его, чтобы построить
// EditManifest на экспорт — рендерится строго то, что лежит на таймлайне.
type tlClip struct {
	SourceURL string            `json:"sourceUrl"`
	TrimStart float64           `json:"trimStart"`
	TrimEnd   float64           `json:"trimEnd"`
	Start     float64           `json:"start"`
	Color     *models.ClipColor `json:"color,omitempty"`
	Audio     *models.ClipAudio `json:"audio,omitempty"`
}

type tlTrack struct {
	Kind  string   `json:"kind"` // "video" | "audio"
	Clips []tlClip `json:"clips"`
}

type tlTimeline struct {
	Tracks []tlTrack `json:"tracks"`
}

// loadOwnedProject загружает проект и проверяет, что он принадлежит текущему юзеру.
// Возвращает (nil, true) если ответ уже записан (ошибка/forbidden).
func (h *Handlers) loadOwnedProject(w http.ResponseWriter, r *http.Request) (*models.EditorProject, bool) {
	id := chi.URLParam(r, "id")
	p, err := h.Store.GetEditorProject(r.Context(), id)
	if handleStoreErr(w, err) {
		return nil, true
	}
	if p.UserID != authpkg.UserID(r) {
		writeError(w, http.StatusForbidden, "forbidden", "not your project")
		return nil, true
	}
	return p, false
}

// POST /api/editor/projects { title?, sourceVideoId?, isRemix? }
func (h *Handlers) CreateEditorProject(w http.ResponseWriter, r *http.Request) {
	if authpkg.Role(r) == "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admins cannot edit channel videos")
		return
	}
	uid := authpkg.UserID(r)

	var req struct {
		Title         string  `json:"title"`
		SourceVideoID *string `json:"sourceVideoId"`
		IsRemix       bool    `json:"isRemix"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	proj := models.EditorProject{
		UserID:        uid,
		Title:         clampString(req.Title, maxTitleLen),
		SourceVideoID: req.SourceVideoID,
		IsRemix:       req.IsRemix,
	}

	// Если проект создаётся на основе видео — проверяем права и засеваем таймлайн.
	if req.SourceVideoID != nil && *req.SourceVideoID != "" {
		video, err := h.Store.GetVideo(r.Context(), *req.SourceVideoID)
		if handleStoreErr(w, err) {
			return
		}
		owner, err := h.Store.OwnerOfVideo(r.Context(), video.ID)
		if handleStoreErr(w, err) {
			return
		}
		own := owner == uid
		if req.IsRemix {
			// «Создать свою версию» чужого ролика разрешена только при allow_remix.
			if !own && !video.AllowRemix {
				writeError(w, http.StatusForbidden, "remix_not_allowed",
					"автор не разрешил создавать версии этого видео")
				return
			}
		} else if !own {
			// Обычный монтаж — только своего видео.
			writeError(w, http.StatusForbidden, "forbidden", "можно монтировать только свои видео")
			return
		}
		if _, ok := h.localFileFromURL(video.VideoURL); !ok {
			writeError(w, http.StatusBadRequest, "no_local_file",
				"исходное видео не хранится локально и не может быть смонтировано")
			return
		}
		if proj.Title == "" {
			if req.IsRemix {
				proj.Title = "Моя версия: " + video.Title
			} else {
				proj.Title = "Монтаж: " + video.Title
			}
		}
		proj.Timeline = buildInitialTimeline(video)
	}

	created, err := h.Store.CreateEditorProject(r.Context(), proj)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GET /api/editor/projects — список проектов пользователя.
func (h *Handlers) ListEditorProjects(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListEditorProjects(r.Context(), authpkg.UserID(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// GET /api/editor/projects/{id}
func (h *Handlers) GetEditorProject(w http.ResponseWriter, r *http.Request) {
	p, done := h.loadOwnedProject(w, r)
	if done {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// PATCH /api/editor/projects/{id} { title?, timeline? } — автосейв/ручное сохранение.
func (h *Handlers) UpdateEditorProject(w http.ResponseWriter, r *http.Request) {
	p, done := h.loadOwnedProject(w, r)
	if done {
		return
	}
	var req struct {
		Title    *string         `json:"title"`
		Timeline json.RawMessage `json:"timeline"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.Title != nil {
		t := clampString(*req.Title, maxTitleLen)
		req.Title = &t
	}
	if len(req.Timeline) > 0 {
		// Базовая валидация JSON timeline (защита от мусора в JSONB).
		var tl tlTimeline
		if err := json.Unmarshal(req.Timeline, &tl); err != nil {
			writeError(w, http.StatusBadRequest, "validation", "invalid timeline json")
			return
		}
	}
	updated, err := h.Store.UpdateEditorProject(r.Context(), p.ID, req.Title, req.Timeline)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/editor/projects/{id}
func (h *Handlers) DeleteEditorProject(w http.ResponseWriter, r *http.Request) {
	p, done := h.loadOwnedProject(w, r)
	if done {
		return
	}
	if err := h.Store.DeleteEditorProject(r.Context(), p.ID); handleStoreErr(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exportProjectReq — параметры экспорта (метаданные нового видео + настройки рендера).
type exportProjectReq struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Visibility  string   `json:"visibility"`
	Tags        []string `json:"tags"`
	AllowRemix  bool     `json:"allowRemix"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	FPS         int      `json:"fps"`
	Format      string   `json:"format"`
	Quality     string   `json:"quality"`
}

// POST /api/editor/projects/{id}/export
//
// Собирает EditManifest из сохранённого timeline проекта (рендерится строго то,
// что на таймлайне), проверяет легальность всех источников, создаёт НОВОЕ видео
// (private, файл появится после рендера) и ставит задачу серверного рендера.
func (h *Handlers) ExportEditorProject(w http.ResponseWriter, r *http.Request) {
	p, done := h.loadOwnedProject(w, r)
	if done {
		return
	}
	uid := authpkg.UserID(r)

	// Анти-дабл: пока активна задача рендера — повторный экспорт запрещён.
	if p.Status == models.ProjectExporting && p.LastRenderJobID != nil {
		if job, err := h.Store.GetRenderJob(r.Context(), *p.LastRenderJobID); err == nil &&
			(job.Status == models.RenderQueued || job.Status == models.RenderProcessing) {
			writeError(w, http.StatusConflict, "export_in_progress", "экспорт уже выполняется")
			return
		}
	}

	var req exportProjectReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	manifest, msg, ok := h.manifestFromTimeline(r, p.Timeline, req)
	if !ok {
		writeError(w, http.StatusBadRequest, "validation", msg)
		return
	}

	// Канал текущего пользователя — на него ложится результат.
	channel, err := h.Store.GetChannelByOwner(r.Context(), uid)
	if handleStoreErr(w, err) {
		return
	}

	// Атрибуция: если проект — производное от другого видео, проставляем источник.
	var sourceVideoID, sourceChannelID *string
	if p.SourceVideoID != nil && *p.SourceVideoID != "" {
		if src, gerr := h.Store.GetVideo(r.Context(), *p.SourceVideoID); gerr == nil {
			sourceVideoID = &src.ID
			sc := src.ChannelID
			sourceChannelID = &sc
		}
	}

	title := clampString(req.Title, maxTitleLen)
	if title == "" {
		title = clampString(p.Title, maxTitleLen)
	}
	if title == "" {
		title = "Видео из редактора"
	}
	visibility := normalizeVisibility(req.Visibility)
	category := req.Category
	if category == "" {
		category = "other"
	}

	// Видео создаётся СНАЧАЛА приватным и без файла — он появится после рендера.
	result, err := h.Store.CreateVideo(r.Context(), models.Video{
		ChannelID:       channel.ID,
		Title:           title,
		Description:     clampString(req.Description, maxDescLen),
		ThumbnailURL:    "",
		VideoURL:        "",
		DurationSec:     manifestDuration(manifest),
		Category:        category,
		Visibility:      "private",
		Tags:            normalizeTags(req.Tags),
		AllowRemix:      req.AllowRemix,
		SourceVideoID:   sourceVideoID,
		SourceChannelID: sourceChannelID,
	})
	if handleStoreErr(w, err) {
		return
	}

	manifest.Title = title
	manifest.Visibility = visibility

	job, err := h.Store.CreateRenderJob(r.Context(), &result.ID, uid, manifest)
	if handleStoreErr(w, err) {
		// Откатываем «пустое» видео, чтобы не плодить мусор.
		_ = h.Store.DeleteVideo(r.Context(), result.ID)
		return
	}
	if err := h.Store.SetEditorProjectExport(r.Context(), p.ID, job.ID, result.ID); err != nil {
		// Не критично для запуска рендера — логируем через handleStoreErr-стиль.
		writeError(w, http.StatusInternalServerError, "internal", "не удалось привязать задачу к проекту")
		return
	}

	h.Renderer.Render(job.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":           job,
		"resultVideoId": result.ID,
	})
}

// POST /api/videos/{id}/remix-permission { allow }
// Автор включает/выключает разрешение «Создать свою версию» для своего видео.
func (h *Handlers) SetVideoRemixPermission(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "id")
	owner, err := h.Store.OwnerOfVideo(r.Context(), videoID)
	if handleStoreErr(w, err) {
		return
	}
	if owner != authpkg.UserID(r) {
		writeError(w, http.StatusForbidden, "forbidden", "это не ваше видео")
		return
	}
	var req struct {
		Allow bool `json:"allow"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := h.Store.SetVideoAllowRemix(r.Context(), videoID, req.Allow); handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"allowRemix": req.Allow})
}

// ---------------------------- helpers ----------------------------

// manifestFromTimeline строит EditManifest из сохранённого timeline и параметров
// экспорта, валидируя и авторизуя КАЖДЫЙ источник клипа (анти-SSRF + allow_remix).
func (h *Handlers) manifestFromTimeline(r *http.Request, timeline json.RawMessage, req exportProjectReq) (models.EditManifest, string, bool) {
	var tl tlTimeline
	if err := json.Unmarshal(timeline, &tl); err != nil {
		return models.EditManifest{}, "повреждённый timeline проекта", false
	}

	var videoClips []tlClip
	var audioClips []tlClip
	for _, tr := range tl.Tracks {
		for _, c := range tr.Clips {
			if tr.Kind == "audio" {
				audioClips = append(audioClips, c)
			} else {
				videoClips = append(videoClips, c)
			}
		}
	}
	if len(videoClips) == 0 {
		return models.EditManifest{}, "на таймлайне нет ни одного видеоклипа", false
	}
	sort.SliceStable(videoClips, func(i, j int) bool { return videoClips[i].Start < videoClips[j].Start })
	sort.SliceStable(audioClips, func(i, j int) bool { return audioClips[i].Start < audioClips[j].Start })

	uid := authpkg.UserID(r)
	authorize := func(url string) (string, bool) {
		if _, ok := h.localFileFromURL(url); !ok {
			return "ссылка клипа должна указывать на загруженный файл (/uploads/...)", false
		}
		ok, err := h.Store.AuthorizeClipSource(r.Context(), uid, url)
		if err != nil || !ok {
			return "нет прав использовать один из источников клипа (нужно своё видео или разрешённый ремикс)", false
		}
		return "", true
	}

	m := models.EditManifest{
		Width:   clampInt(req.Width, 0, 1920),
		Height:  clampInt(req.Height, 0, 1080),
		FPS:     normalizeFPS(req.FPS),
		Format:  normalizeFormat(req.Format),
		Quality: normalizeQuality(req.Quality),
	}
	for _, c := range videoClips {
		if msg, ok := authorize(c.SourceURL); !ok {
			return models.EditManifest{}, msg, false
		}
		if c.TrimStart < 0 || c.TrimEnd <= c.TrimStart {
			return models.EditManifest{}, "некорректный диапазон обрезки клипа", false
		}
		m.Clips = append(m.Clips, models.EditClip{
			SourceURL: c.SourceURL,
			TrimStart: c.TrimStart,
			TrimEnd:   c.TrimEnd,
			Color:     c.Color,
			Audio:     c.Audio,
		})
	}
	for _, c := range audioClips {
		if msg, ok := authorize(c.SourceURL); !ok {
			return models.EditManifest{}, msg, false
		}
		if c.TrimStart < 0 || c.TrimEnd <= c.TrimStart {
			return models.EditManifest{}, "некорректный диапазон обрезки аудиоклипа", false
		}
		vol, fin, fout := 1.0, 0.0, 0.0
		if c.Audio != nil {
			vol, fin, fout = c.Audio.Volume, c.Audio.FadeIn, c.Audio.FadeOut
			if c.Audio.Muted {
				vol = 0
			}
		}
		m.AudioClips = append(m.AudioClips, models.AudioClip{
			SourceURL: c.SourceURL,
			TrimStart: c.TrimStart,
			TrimEnd:   c.TrimEnd,
			Start:     c.Start,
			Volume:    vol,
			FadeIn:    fin,
			FadeOut:   fout,
		})
	}
	return m, "", true
}

// buildInitialTimeline создаёт стартовую раскладку из одного клипа на всю длину видео.
func buildInitialTimeline(v *models.Video) json.RawMessage {
	dur := float64(v.DurationSec)
	if dur <= 0 {
		dur = 60 // запас, если длительность неизвестна; фронт уточнит по метаданным
	}
	tl := map[string]any{
		"version":  1,
		"duration": dur,
		"tracks": []map[string]any{
			{
				"id":   "video-1",
				"kind": "video",
				"clips": []map[string]any{
					{
						"id":        "clip-1",
						"sourceUrl": v.VideoURL,
						"trimStart": 0,
						"trimEnd":   dur,
						"start":     0,
					},
				},
			},
			{"id": "audio-1", "kind": "audio", "clips": []any{}},
		},
	}
	raw, _ := json.Marshal(tl)
	return raw
}

func manifestDuration(m models.EditManifest) int {
	var total float64
	for _, c := range m.Clips {
		total += c.TrimEnd - c.TrimStart
	}
	if total < 1 {
		return 1
	}
	return int(total + 0.5)
}

func clampString(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max]
	}
	return s
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func normalizeVisibility(v string) string {
	if v == "public" {
		return "public"
	}
	return "private"
}

func normalizeFormat(f string) string {
	if strings.ToLower(f) == "webm" {
		return "webm"
	}
	return "mp4"
}

func normalizeQuality(q string) string {
	switch q {
	case "low", "high":
		return q
	default:
		return "medium"
	}
}

func normalizeFPS(fps int) int {
	switch fps {
	case 24, 60:
		return fps
	default:
		return 30
	}
}
