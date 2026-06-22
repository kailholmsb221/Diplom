// Package server собирает chi.Router из набора хендлеров.
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	authpkg "videohub/internal/auth"
	"videohub/internal/config"
	"videohub/internal/handlers"
)

func NewRouter(h *handlers.Handlers, cfg config.Config) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// JWT parsed-but-optional: дальше отдельные группы применяют RequireAuth/RequireAdmin.
	r.Use(authpkg.Middleware(cfg.JWTSecret))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Статика: /uploads/MP4/*, /uploads/PNG/*, /uploads/ADS/*
	staticAbs, _ := filepath.Abs(cfg.UploadsDir)
	_ = os.MkdirAll(filepath.Join(staticAbs, "MP4"), 0o755)
	_ = os.MkdirAll(filepath.Join(staticAbs, "PNG"), 0o755)
	_ = os.MkdirAll(filepath.Join(staticAbs, "ADS"), 0o755)
	_ = os.MkdirAll(filepath.Join(staticAbs, "SUBTITLES"), 0o755)
	_ = os.MkdirAll(filepath.Join(staticAbs, "TMP"), 0o755) // части multipart-загрузок
	_ = os.MkdirAll(filepath.Join(staticAbs, "HLS"), 0o755) // результат экспорта (HLS)
	fs := http.StripPrefix("/uploads/", http.FileServer(http.Dir(staticAbs)))
	r.Get("/uploads/{subdir}/{file}", h.ServeUpload)
	r.Handle("/uploads/*", fs)

	r.Route("/api", func(r chi.Router) {
		// ---------- Авторизация (всё публично) ----------
		r.Post("/auth/register", h.AuthRegister)
		r.Post("/auth/register/verify", h.AuthRegisterVerify)
		r.Post("/auth/login", h.AuthLogin)
		r.Get("/auth/me", h.AuthMe)

		// ---------- Публичные read-only эндпоинты ----------
		r.Get("/videos", h.ListVideos)
		r.Get("/videos/recommended", h.Recommended)
		r.Get("/videos/{id}", h.GetVideo)
		r.Get("/videos/{id}/recommendations", h.Recommendations)
		r.Get("/videos/{id}/comments", h.ListComments)
		r.Post("/videos/{id}/views", h.RegisterView) // считаем просмотры без логина

		r.Get("/channels", h.ListChannels)
		r.Get("/channels/{id}", h.GetChannel)
		r.Get("/channels/{id}/stats", h.ChannelStats)
		r.Get("/channels/by-owner/{userId}", h.GetChannelByOwner)

		r.Get("/users", h.ListUsers)
		r.Get("/users/{id}", h.GetUser)

		// Реклама — публично читаем активную.
		r.Get("/ads/active", h.ActiveAd)

		// Транскрипция / субтитры — публично.
		r.Get("/videos/{id}/transcript", h.GetTranscript)
		r.Get("/videos/{id}/subtitles", h.GetSubtitles)

		// ---------- Требуют авторизации ----------
		r.Group(func(r chi.Router) {
			r.Use(authpkg.RequireAuth)

			r.Post("/videos", h.CreateVideo)
			r.Delete("/videos/{id}", h.DeleteVideo)

			// ---------- Видеоредактор: загрузка ----------
			r.Post("/videos/upload-url", h.UploadURL)
			r.Post("/videos/multipart/init", h.MultipartInit)
			r.Put("/videos/multipart/{uploadId}/part/{partNumber}", h.MultipartPart)
			r.Post("/videos/multipart/complete", h.MultipartComplete)

			// ---------- Видеоредактор: экспорт (рендер) ----------
			r.Post("/videos/{id}/export", h.ExportVideo)
			r.Get("/videos/{id}/export/{jobId}", h.GetExportJob)
			r.Get("/videos/{id}/export/{jobId}/progress", h.ExportProgress)

			// ---------- Видеоредактор: проекты (черновики монтажа) ----------
			r.Post("/editor/projects", h.CreateEditorProject)
			r.Get("/editor/projects", h.ListEditorProjects)
			r.Get("/editor/projects/{id}", h.GetEditorProject)
			r.Patch("/editor/projects/{id}", h.UpdateEditorProject)
			r.Delete("/editor/projects/{id}", h.DeleteEditorProject)
			r.Post("/editor/projects/{id}/export", h.ExportEditorProject)
			r.Get("/editor/projects/{id}/export/{jobId}", h.GetExportJob)
			r.Get("/editor/projects/{id}/export/{jobId}/progress", h.ExportProgress)

			// Разрешение на ремикс («Создать свою версию») — управляет автор.
			r.Post("/videos/{id}/remix-permission", h.SetVideoRemixPermission)
			r.Get("/videos/{id}/reaction", h.GetReaction)
			r.Post("/videos/{id}/reaction", h.SetReaction)
			r.Post("/videos/{id}/comments", h.AddComment)
			r.Delete("/comments/{id}", h.DeleteComment)

			r.Patch("/channels/{id}", h.UpdateChannel)
			r.Get("/channels/{id}/subscribed", h.IsSubscribed)
			r.Post("/channels/{id}/subscribe", h.Subscribe)
			r.Delete("/channels/{id}/subscribe", h.Unsubscribe)

			r.Get("/subscriptions", h.ListSubscriptions)

			r.Get("/users/me", h.GetMe)
			r.Patch("/users/me", h.UpdateMe)

			r.Post("/upload/video", h.UploadVideo)
			r.Post("/upload/image", h.UploadImage)
			r.Post("/upload/audio", h.UploadAudio)

			// Монетизация
			r.Post("/premium/buy", h.BuyPremium)
			r.Get("/transactions/me", h.MyTransactions)
			r.Get("/channels/me", h.MyChannel)
			r.Post("/channels/me/payout", h.PayoutMyChannel)

			// Транскрипция — повторный запуск.
			r.Post("/videos/{id}/transcribe", h.StartTranscribe)
			r.Post("/videos/{id}/translate", h.StartTranslate)
		})

		// ---------- Только админ ----------
		r.Group(func(r chi.Router) {
			r.Use(authpkg.RequireAuth, authpkg.RequireAdmin)

			r.Get("/admin/stats", h.PlatformStats)
			r.Get("/admin/comments", h.AdminListComments)
			r.Post("/admin/users/{id}/block", h.AdminBlockUser)
			r.Post("/admin/channels/{id}/stats", h.AdminAdjustStats)
			r.Post("/admin/videos/{id}/stats", h.AdminAdjustVideoStats)

			// Реклама — управление
			r.Get("/admin/ads", h.AdminListAds)
			r.Post("/admin/ads", h.AdminCreateAd)
			r.Patch("/admin/ads/{id}", h.AdminUpdateAd)
			r.Delete("/admin/ads/{id}", h.AdminDeleteAd)
			r.Post("/upload/ad", h.UploadAd)

			// Финансы
			r.Get("/admin/transactions", h.AdminListTransactions)
			r.Get("/admin/users/premium", h.AdminListPremium)
			r.Patch("/admin/users/{id}/premium", h.AdminSetPremium)
			r.Patch("/admin/channels/{id}/balance", h.AdminAdjustBalance)
		})
	})

	if cfg.StaticDir != "" {
		if staticAbs, err := filepath.Abs(cfg.StaticDir); err == nil {
			if info, statErr := os.Stat(staticAbs); statErr == nil && info.IsDir() {
				r.Get("/*", spaHandler(staticAbs))
			}
		}
	}

	return r
}

func spaHandler(staticAbs string) http.HandlerFunc {
	indexPath := filepath.Join(staticAbs, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		filePath := filepath.Join(staticAbs, cleanPath)

		rel, err := filepath.Rel(staticAbs, filePath)
		if err != nil || strings.HasPrefix(rel, "..") {
			http.ServeFile(w, r, indexPath)
			return
		}

		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, filePath)
			return
		}

		http.ServeFile(w, r, indexPath)
	}
}
