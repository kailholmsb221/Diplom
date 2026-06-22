package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	UploadsDir    string
	StaticDir     string
	PublicBaseURL string
	CORSOrigins   []string
	MaxVideoBytes int64
	MaxImageBytes int64
	DefaultUserID string
	ReseedOnStart bool

	JWTSecret      []byte
	JWTTTLHours    int
	AdminUsername  string
	AdminPassword  string
	AuthCodeTTLMin int
	// AuthExposeCode = true означает, что backend будет возвращать сгенерированный код
	// прямо в ответе API (для демонстрации без SMTP). В проде должно быть false.
	AuthExposeCode bool

	// --- Транскрипция (Python worker) ---
	// Если PythonPath или WorkerPath пусты / файл не найден — фича отключается,
	// но видео остаются рабочими (TranscriptStatus = NOT_STARTED).
	PythonPath     string
	WorkerPath     string
	WhisperModel   string // tiny / base / small / medium / large-v3
	WhisperCompute string // int8 / float32 / ...

	// --- Видеоредактор: серверный рендер (экспорт) ---
	// Если RenderWorkerPath пуст / файл не найден — экспорт сразу падает в FAILED,
	// но остальной функционал редактора (загрузка, метаданные) работает.
	RenderWorkerPath string
	FFmpegPath       string
	FFprobePath      string
}

func Load() Config {
	return Config{
		HTTPAddr:      getHTTPAddr(),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://videohub:videohub@localhost:5432/videohub?sslmode=disable"),
		UploadsDir:    getEnv("UPLOADS_DIR", "./uploads"),
		StaticDir:     getEnv("STATIC_DIR", ""),
		PublicBaseURL: getPublicBaseURL(),
		CORSOrigins:   splitCSV(getEnv("CORS_ORIGINS", "http://localhost:5173")),
		MaxVideoBytes: getEnvInt64("MAX_VIDEO_BYTES", 500*1024*1024),
		MaxImageBytes: getEnvInt64("MAX_IMAGE_BYTES", 10*1024*1024),
		DefaultUserID: getEnv("DEFAULT_USER_ID", "11111111-1111-1111-1111-111111111111"),
		ReseedOnStart: getEnvBool("RESEED_ON_START", false),

		JWTSecret:      []byte(getEnv("JWT_SECRET", "videohub-dev-secret-change-me")),
		JWTTTLHours:    int(getEnvInt64("JWT_TTL_HOURS", 24*7)),
		AdminUsername:  getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:  getEnv("ADMIN_PASSWORD", "admin1"),
		AuthCodeTTLMin: int(getEnvInt64("AUTH_CODE_TTL_MIN", 10)),
		AuthExposeCode: getEnvBool("AUTH_EXPOSE_CODE", true),

		PythonPath:     getEnv("PYTHON_BIN", "python3"),
		WorkerPath:     getEnv("WHISPER_WORKER", "/app/worker/transcribe.py"),
		WhisperModel:   getEnv("WHISPER_MODEL", "small"),
		WhisperCompute: getEnv("WHISPER_COMPUTE", "int8"),

		RenderWorkerPath: getEnv("RENDER_WORKER", "/app/worker/render.py"),
		FFmpegPath:       getEnv("FFMPEG_BIN", "ffmpeg"),
		FFprobePath:      getEnv("FFPROBE_BIN", "ffprobe"),
	}
}

func getHTTPAddr() string {
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		return v
	}
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

func getPublicBaseURL() string {
	if v := os.Getenv("PUBLIC_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if host := os.Getenv("RENDER_EXTERNAL_HOSTNAME"); host != "" {
		return "https://" + strings.TrimRight(host, "/")
	}
	return "http://localhost:8080"
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
