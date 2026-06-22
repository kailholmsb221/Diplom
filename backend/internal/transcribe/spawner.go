// Package transcribe запускает Python-worker для транскрипции видео.
package transcribe

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"videohub/internal/config"
	"videohub/internal/store"
)

// ErrWorkerUnavailable возвращается, если Python или worker не найдены.
var ErrWorkerUnavailable = errors.New("transcription worker is not configured")

// Spawner запускает worker в фоне. Один экземпляр на приложение.
type Spawner struct {
	Cfg   config.Config
	Store *store.Store
}

func New(cfg config.Config, st *store.Store) *Spawner {
	return &Spawner{Cfg: cfg, Store: st}
}

// Available — true, если worker-скрипт существует и Python доступен.
// Используется фронтом косвенно: если воркер недоступен, backend сразу пишет
// FAILED и пользователь видит ошибку.
func (s *Spawner) Available() bool {
	if s.Cfg.PythonPath == "" || s.Cfg.WorkerPath == "" {
		return false
	}
	if _, err := os.Stat(s.Cfg.WorkerPath); err != nil {
		return false
	}
	if _, err := exec.LookPath(s.Cfg.PythonPath); err != nil {
		return false
	}
	return true
}

// Transcribe запускает полный цикл: распознавание + перевод. Не блокирует.
// videoFilePath — абсолютный путь к загруженному видео на диске.
func (s *Spawner) Transcribe(videoID, videoFilePath string) {
	go s.run([]string{videoID, videoFilePath}, videoID, "transcribe")
}

// Translate запускает только переводы (оригинал должен быть уже готов).
func (s *Spawner) Translate(videoID string) {
	go s.run([]string{"translate", videoID}, videoID, "translate")
}

func (s *Spawner) run(args []string, videoID, kind string) {
	if !s.Available() {
		msg := fmt.Sprintf("python worker unavailable (PYTHON_BIN=%q WORKER=%q)",
			s.Cfg.PythonPath, s.Cfg.WorkerPath)
		log.Printf("transcribe[%s] %s: %s", kind, videoID, msg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Store.SetVideoTranscriptStatus(ctx, videoID, "FAILED", msg)
		return
	}

	cmd := exec.Command(s.Cfg.PythonPath, append([]string{s.Cfg.WorkerPath}, args...)...)
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+s.Cfg.DatabaseURL,
		"UPLOADS_DIR="+absOrSelf(s.Cfg.UploadsDir),
		"WHISPER_MODEL="+s.Cfg.WhisperModel,
		"WHISPER_COMPUTE="+s.Cfg.WhisperCompute,
		"PYTHONUNBUFFERED=1",
	)
	// Поток stdout/stderr пишем в системный лог backend'а.
	cmd.Stdout = logWriter{prefix: "transcribe[" + kind + "] " + videoID + ":"}
	cmd.Stderr = cmd.Stdout

	log.Printf("transcribe[%s] %s: spawning %s %s",
		kind, videoID, s.Cfg.PythonPath, strings.Join(append([]string{s.Cfg.WorkerPath}, args...), " "))
	if err := cmd.Run(); err != nil {
		log.Printf("transcribe[%s] %s: exit error: %v", kind, videoID, err)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Worker сам пишет FAILED при внутренних ошибках; но если он крашнулся —
		// гарантированно ставим FAILED здесь.
		_ = s.Store.SetVideoTranscriptStatus(ctx, videoID, "FAILED", err.Error())
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
