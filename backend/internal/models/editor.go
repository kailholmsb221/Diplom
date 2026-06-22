package models

import (
	"encoding/json"
	"time"
)

// Статусы рендера (экспорта) видео.
const (
	RenderQueued     = "queued"
	RenderProcessing = "processing"
	RenderCompleted  = "completed"
	RenderFailed     = "failed"
)

// Статусы сессии чанковой загрузки.
const (
	UploadOpen      = "open"
	UploadCompleted = "completed"
	UploadAborted   = "aborted"
)

// UploadSession — состояние multipart-загрузки одного файла.
type UploadSession struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId"`
	FileName      string    `json:"fileName"`
	Ext           string    `json:"ext"`
	ContentType   string    `json:"contentType"`
	SizeBytes     int64     `json:"sizeBytes"`
	TotalParts    int       `json:"totalParts"`
	ReceivedParts int       `json:"receivedParts"`
	ReceivedBytes int64     `json:"receivedBytes"`
	Status        string    `json:"status"`
	FinalPath     string    `json:"-"`
	FinalURL      string    `json:"finalUrl,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// ClipColor — параметры цветокоррекции клипа. Нейтральные значения дают
// картинку без изменений (см. neutral-проверку в render.py: фильтр не добавляется).
// Диапазоны согласованы с CSS-превью на фронте и фильтрами FFmpeg (eq/hue/unsharp).
type ClipColor struct {
	Brightness float64 `json:"brightness"` // -1..1   (eq=brightness, 0 = норма)
	Contrast   float64 `json:"contrast"`   // 0..2    (eq=contrast,  1 = норма)
	Saturation float64 `json:"saturation"` // 0..3    (eq=saturation,1 = норма)
	Exposure   float64 `json:"exposure"`   // 0.1..3  (eq=gamma,     1 = норма)
	Temperature float64 `json:"temperature"` // -100..100 (тёплый/холодный, 0 = норма)
	Hue        float64 `json:"hue"`        // -180..180 градусов (hue=h, 0 = норма)
	Highlights float64 `json:"highlights"` // -1..1   (curves светов, 0 = норма)
	Shadows    float64 `json:"shadows"`    // -1..1   (curves теней, 0 = норма)
	Sharpness  float64 `json:"sharpness"`  // 0..2    (unsharp, 0 = норма)
	Opacity    float64 `json:"opacity"`    // 0..1    (1 = норма)
}

// ClipAudio — настройки звука клипа.
type ClipAudio struct {
	Muted   bool    `json:"muted"`   // полностью убрать звук дорожки клипа
	Volume  float64 `json:"volume"`  // 0..2 (1 = норма)
	FadeIn  float64 `json:"fadeIn"`  // сек нарастания в начале клипа
	FadeOut float64 `json:"fadeOut"` // сек затухания в конце клипа
}

// EditClip — один клип в монтажной раскладке.
// TrimStart/TrimEnd — секунды от начала исходника SourceURL.
type EditClip struct {
	SourceURL string     `json:"sourceUrl"`
	TrimStart float64    `json:"trimStart"`
	TrimEnd   float64    `json:"trimEnd"`
	Color     *ClipColor `json:"color,omitempty"`
	Audio     *ClipAudio `json:"audio,omitempty"`
}

// AudioClip — клип на отдельной аудиодорожке (добавленная музыка / озвучка с микрофона).
// Start — позиция начала на общем таймлайне (сек); render.py задерживает поток на adelay.
type AudioClip struct {
	SourceURL string  `json:"sourceUrl"`
	TrimStart float64 `json:"trimStart"`
	TrimEnd   float64 `json:"trimEnd"`
	Start     float64 `json:"start"`
	Volume    float64 `json:"volume"`
	FadeIn    float64 `json:"fadeIn"`
	FadeOut   float64 `json:"fadeOut"`
}

// EditManifest — то, что уходит на экспорт: упорядоченный список клипов + параметры вывода.
// render.py транслирует его в FFmpeg-команду (-ss/-to + per-clip фильтры + concat) → mp4.
type EditManifest struct {
	Title   string     `json:"title,omitempty"`
	Clips   []EditClip `json:"clips"`
	Width   int        `json:"width,omitempty"`   // целевое разрешение, 0 = 1280
	Height  int        `json:"height,omitempty"`  // 0 = 720
	FPS        int    `json:"fps,omitempty"`        // 0 = 30
	Format     string `json:"format,omitempty"`     // "mp4" (по умолчанию) | "webm"
	Quality    string `json:"quality,omitempty"`    // "low"|"medium"|"high" → CRF
	Visibility string `json:"visibility,omitempty"` // целевая видимость результата: public|private

	AudioClips []AudioClip `json:"audioClips,omitempty"` // доп. аудиодорожки (музыка/озвучка)
}

// Статусы проекта редактора.
const (
	ProjectDraft     = "draft"
	ProjectExporting = "exporting"
	ProjectExported  = "exported"
)

// EditorProject — НЕразрушающий черновик монтажа. Оригиналы не меняются.
type EditorProject struct {
	ID              string          `json:"id"`
	UserID          string          `json:"userId"`
	Title           string          `json:"title"`
	SourceVideoID   *string         `json:"sourceVideoId,omitempty"`
	IsRemix         bool            `json:"isRemix"`
	Timeline        json.RawMessage `json:"timeline"`
	Status          string          `json:"status"`
	LastRenderJobID *string         `json:"lastRenderJobId,omitempty"`
	ResultVideoID   *string         `json:"resultVideoId,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// RenderJob — задача серверного рендера по EditManifest.
type RenderJob struct {
	ID        string       `json:"id"`
	VideoID   *string      `json:"videoId,omitempty"`
	UserID    string       `json:"userId"`
	Status    string       `json:"status"`
	Progress  int          `json:"progress"`
	Manifest  EditManifest `json:"manifest"`
	OutputURL string       `json:"outputUrl,omitempty"`
	Error     string       `json:"error,omitempty"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}
