// Package render запускает Python-worker (render.py) для серверного экспорта
// видео по EditManifest. По смыслу — локальный аналог Celery-задачи render_video:
// строит FFmpeg-команду (-ss/-t + concat) и нарезает HLS-сегменты в uploads/HLS.
package render

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"videohub/internal/config"
	"videohub/internal/models"
	"videohub/internal/store"
)

// Spawner запускает render.py в фоне. Один экземпляр на приложение.
type Spawner struct {
	Cfg   config.Config
	Store *store.Store
}

func New(cfg config.Config, st *store.Store) *Spawner {
	return &Spawner{Cfg: cfg, Store: st}
}

// Available — true, если render-скрипт существует и Python доступен.
func (s *Spawner) Available() bool {
	if s.Cfg.PythonPath == "" || s.Cfg.RenderWorkerPath == "" {
		return false
	}
	if _, err := os.Stat(s.Cfg.RenderWorkerPath); err != nil {
		return false
	}
	if _, err := exec.LookPath(s.Cfg.PythonPath); err != nil {
		return false
	}
	return true
}

// Render запускает рендер задачи jobID. Не блокирует.
func (s *Spawner) Render(jobID string) {
	go s.run(jobID)
}

func (s *Spawner) run(jobID string) {
	if !s.Available() {
		msg := fmt.Sprintf("render worker unavailable (PYTHON_BIN=%q RENDER_WORKER=%q)",
			s.Cfg.PythonPath, s.Cfg.RenderWorkerPath)
		log.Printf("render %s: %s", jobID, msg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Store.SetRenderJobDone(ctx, jobID, models.RenderFailed, "", msg)
		return
	}

	cmd := exec.Command(s.Cfg.PythonPath, s.Cfg.RenderWorkerPath, jobID)
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+s.Cfg.DatabaseURL,
		"UPLOADS_DIR="+absOrSelf(s.Cfg.UploadsDir),
		"PUBLIC_BASE_URL="+strings.TrimRight(s.Cfg.PublicBaseURL, "/"),
		"FFMPEG_BIN="+s.Cfg.FFmpegPath,
		"FFPROBE_BIN="+s.Cfg.FFprobePath,
		"PYTHONUNBUFFERED=1",
	)
	cmd.Stdout = logWriter{prefix: "render " + jobID + ":"}
	cmd.Stderr = cmd.Stdout

	log.Printf("render %s: spawning %s %s %s", jobID, s.Cfg.PythonPath, s.Cfg.RenderWorkerPath, jobID)
	if err := cmd.Run(); err != nil {
		log.Printf("render %s: exit error: %v", jobID, err)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// render.py сам помечает failed при внутренних ошибках; здесь — страховка
		// на случай краша процесса. Перетираем только если задача ещё не завершена.
		if j, gerr := s.Store.GetRenderJob(ctx, jobID); gerr == nil &&
			j.Status != models.RenderCompleted && j.Status != models.RenderFailed {
			_ = s.Store.SetRenderJobDone(ctx, jobID, models.RenderFailed, "", err.Error())
		}
	}
}

func absOrSelf(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// logWriter — io.Writer, пишущий каждую запись в стандартный log.
type logWriter struct{ prefix string }

func (w logWriter) Write(p []byte) (int, error) {
	log.Printf("%s %s", w.prefix, strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
