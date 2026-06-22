package handlers

import (
	"encoding/json"
	"testing"

	"videohub/internal/config"
	"videohub/internal/models"
)

func testHandlers() *Handlers {
	return &Handlers{Cfg: config.Config{UploadsDir: "./uploads", PublicBaseURL: "http://localhost:8080"}}
}

// validateManifest — первая линия защиты экспорта: только локальные /uploads-файлы
// (анти-SSRF) и корректные диапазоны обрезки.
func TestValidateManifest_RejectsExternalSources(t *testing.T) {
	h := testHandlers()
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		{"external youtube", "https://www.youtube.com/watch?v=abc", false},
		{"external file", "https://evil.example.com/video.mp4", false},
		{"local upload", "http://localhost:8080/uploads/MP4/clip.mp4", true},
		{"relative upload", "/uploads/MP4/clip.mp4", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := models.EditManifest{Clips: []models.EditClip{{SourceURL: c.url, TrimStart: 0, TrimEnd: 5}}}
			_, ok := h.validateManifest(m)
			if ok != c.ok {
				t.Fatalf("url %q: got ok=%v want %v", c.url, ok, c.ok)
			}
		})
	}
}

func TestValidateManifest_RejectsBadTrim(t *testing.T) {
	h := testHandlers()
	m := models.EditManifest{Clips: []models.EditClip{
		{SourceURL: "/uploads/MP4/a.mp4", TrimStart: 5, TrimEnd: 3}, // end <= start
	}}
	if _, ok := h.validateManifest(m); ok {
		t.Fatal("expected invalid trim range to be rejected")
	}
	if _, ok := h.validateManifest(models.EditManifest{Clips: nil}); ok {
		t.Fatal("expected empty manifest to be rejected")
	}
}

func TestNormalizeExportParams(t *testing.T) {
	if got := normalizeFPS(99); got != 30 {
		t.Errorf("normalizeFPS(99)=%d want 30", got)
	}
	if got := normalizeFPS(60); got != 60 {
		t.Errorf("normalizeFPS(60)=%d want 60", got)
	}
	if got := normalizeQuality("ultra"); got != "medium" {
		t.Errorf("normalizeQuality(ultra)=%q want medium", got)
	}
	if got := normalizeFormat("MKV"); got != "mp4" {
		t.Errorf("normalizeFormat(MKV)=%q want mp4", got)
	}
	if got := normalizeFormat("webm"); got != "webm" {
		t.Errorf("normalizeFormat(webm)=%q want webm", got)
	}
	if got := normalizeVisibility("public"); got != "public" {
		t.Errorf("normalizeVisibility(public)=%q want public", got)
	}
	if got := normalizeVisibility("hacker"); got != "private" {
		t.Errorf("normalizeVisibility(hacker)=%q want private", got)
	}
	if got := clampInt(5000, 0, 1920); got != 1920 {
		t.Errorf("clampInt cap failed: %d", got)
	}
}

func TestManifestDuration(t *testing.T) {
	m := models.EditManifest{Clips: []models.EditClip{
		{TrimStart: 0, TrimEnd: 10},
		{TrimStart: 5, TrimEnd: 8},
	}}
	if got := manifestDuration(m); got != 13 {
		t.Errorf("manifestDuration=%d want 13", got)
	}
}

func TestBuildInitialTimeline(t *testing.T) {
	v := &models.Video{ID: "v1", VideoURL: "/uploads/MP4/a.mp4", DurationSec: 42}
	raw := buildInitialTimeline(v)
	var tl tlTimeline
	if err := json.Unmarshal(raw, &tl); err != nil {
		t.Fatalf("timeline not valid json: %v", err)
	}
	if len(tl.Tracks) == 0 || len(tl.Tracks[0].Clips) != 1 {
		t.Fatalf("expected one seeded video clip, got %+v", tl.Tracks)
	}
	c := tl.Tracks[0].Clips[0]
	if c.SourceURL != v.VideoURL || c.TrimEnd != 42 {
		t.Fatalf("seeded clip wrong: %+v", c)
	}
}
