// Package seed наполняет БД демонстрационными данными.
// Запускается при пустой таблице users или при RESEED_ON_START=true.
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Run проверяет, есть ли уже пользователи; если нет — наполняет БД.
// defaultUserID становится id главного "залогиненного" админа.
func Run(ctx context.Context, pool *pgxpool.Pool, defaultUserID string) error {
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ensureBulkSeed(ctx, pool)
	}
	return reseed(ctx, pool, defaultUserID)
}

// Force всегда наполняет БД (предполагается, что таблицы уже пустые).
func Force(ctx context.Context, pool *pgxpool.Pool, defaultUserID string) error {
	return reseed(ctx, pool, defaultUserID)
}

func ensureBulkSeed(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM videos
		WHERE id::text LIKE 'dd000000-%'
		  AND video_url LIKE '%youtube.com%'
		  AND views_count > 0
		  AND likes_count > 0
		  AND dislikes_count > 0`).Scan(&n); err != nil {
		return err
	}
	if n >= bulkChannelLimit*videosPerBulkChannel {
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := seedBulkChannels(ctx, tx, time.Now().UTC()); err != nil {
		return fmt.Errorf("seed bulk channels: %w", err)
	}
	return tx.Commit(ctx)
}

func reseed(ctx context.Context, pool *pgxpool.Pool, defaultUserID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// ---------------- Users ----------------
	// Все демо-пользователи имеют логин/пароль для входа через /login.
	// Админ: admin / admin1. Остальные: <login> / password.
	type userSeed struct {
		id, username, displayName, email, avatar, bio, role, password string
	}
	users := []userSeed{
		{defaultUserID, "admin", "Администратор", "admin@videohub.local",
			"",
			"Учётка администратора платформы.", "admin", "admin1"},
		{"22222222-2222-2222-2222-222222222222", "tech_guru", "TechGuru", "tech@videohub.local",
			"", "Обзоры гаджетов.", "user", "password"},
		{"33333333-3333-3333-3333-333333333333", "music_master", "MusicMaster", "music@videohub.local",
			"", "Музыкальные клипы.", "user", "password"},
		{"44444444-4444-4444-4444-444444444444", "gamer_pro", "GamerPro", "gamer@videohub.local",
			"", "Прохождения игр.", "user", "password"},
		{"55555555-5555-5555-5555-555555555555", "edu_channel", "EduChannel", "edu@videohub.local",
			"", "Образовательные ролики.", "user", "password"},
		{"66666666-6666-6666-6666-666666666666", "sport_zone", "SportZone", "sport@videohub.local",
			"", "Спорт и тренировки.", "user", "password"},
		{"77777777-7777-7777-7777-777777777777", "ivan", "Иван Петров", "ivan@videohub.local",
			"", "Снимаю видео о технологиях.", "user", "password"},
		{"88888888-8888-8888-8888-888888888888", "cooking", "CookingMaster", "cooking@videohub.local",
			"", "Кулинарные рецепты и лайфхаки.", "user", "password"},
		{"99999999-9999-9999-9999-999999999999", "travel", "TravelDiary", "travel@videohub.local",
			"", "Путешествия и приключения.", "user", "password"},
		{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "art_studio", "ArtStudio", "art@videohub.local",
			"", "Уроки рисования и обзоры выставок.", "user", "password"},
	}
	for _, u := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password for %s: %w", u.username, err)
		}
		h := string(hash)
		if _, err := tx.Exec(ctx, `
			INSERT INTO users(id, username, display_name, email, avatar_url, bio, role, password_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			u.id, u.username, u.displayName, u.email, u.avatar, u.bio, u.role, &h); err != nil {
			return fmt.Errorf("seed users: %w", err)
		}
	}

	// ---------------- Channels (один на пользователя) ----------------
	type chSeed struct {
		id, ownerID, name, handle, desc, avatar, banner string
		subs                                            int64
	}
	channels := []chSeed{
		{"c1c1c1c1-1111-1111-1111-111111111111", defaultUserID, "Канал администратора", "@admin",
			"Официальный канал администратора платформы.",
			"", "", 12400},
		{"c2c2c2c2-2222-2222-2222-222222222222", "22222222-2222-2222-2222-222222222222", "TechGuru", "@tech_guru",
			"Обзоры смартфонов и ноутбуков.",
			"", "", 384200},
		{"c3c3c3c3-3333-3333-3333-333333333333", "33333333-3333-3333-3333-333333333333", "MusicMaster", "@music_master",
			"Клипы, обзоры альбомов.",
			"", "", 1205000},
		{"c4c4c4c4-4444-4444-4444-444444444444", "44444444-4444-4444-4444-444444444444", "GamerPro", "@gamer_pro",
			"Стримы и прохождения.",
			"", "", 762300},
		{"c5c5c5c5-5555-5555-5555-555555555555", "55555555-5555-5555-5555-555555555555", "EduChannel", "@edu_channel",
			"Сложное простыми словами.",
			"", "", 98400},
		{"c6c6c6c6-6666-6666-6666-666666666666", "66666666-6666-6666-6666-666666666666", "SportZone", "@sport_zone",
			"Тренировки и мотивация.",
			"", "", 54120},
		{"c7c7c7c7-7777-7777-7777-777777777777", "77777777-7777-7777-7777-777777777777", "Иван о технологиях", "@ivan",
			"Разработка, обзоры IDE, советы для начинающих.",
			"", "", 12400},
		{"c8c8c8c8-8888-8888-8888-888888888888", "88888888-8888-8888-8888-888888888888", "CookingMaster", "@cooking",
			"Кулинарные рецепты и кухонные лайфхаки.",
			"", "", 76300},
		{"c9c9c9c9-9999-9999-9999-999999999999", "99999999-9999-9999-9999-999999999999", "TravelDiary", "@travel",
			"Путешествия, маршруты и приключения.",
			"", "", 142800},
		{"cacacaca-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "ArtStudio", "@art_studio",
			"Уроки рисования и обзоры выставок.",
			"", "", 31400},
	}
	for _, c := range channels {
		if _, err := tx.Exec(ctx, `
			INSERT INTO channels(id, owner_id, name, handle, description, avatar_url, banner_url, subscribers_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			c.id, c.ownerID, c.name, c.handle, c.desc, c.avatar, c.banner, c.subs); err != nil {
			return fmt.Errorf("seed channels: %w", err)
		}
	}

	// Видео, комментарии, просмотры и реакции НЕ генерируются —
	// они появятся только после реальных загрузок и действий пользователей.
	now := time.Now().UTC()

	// ---------------- Subscriptions ----------------
	subs := []struct {
		subscriberID, channelID string
		daysAgo                 int
	}{
		{defaultUserID, "c2c2c2c2-2222-2222-2222-222222222222", 50},
		{defaultUserID, "c3c3c3c3-3333-3333-3333-333333333333", 45},
		{defaultUserID, "c5c5c5c5-5555-5555-5555-555555555555", 20},
	}
	for _, s := range subs {
		created := now.AddDate(0, 0, -s.daysAgo)
		if _, err := tx.Exec(ctx, `
			INSERT INTO subscriptions(subscriber_id, channel_id, created_at) VALUES ($1, $2, $3)`,
			s.subscriberID, s.channelID, created); err != nil {
			return fmt.Errorf("seed subscriptions: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO subscription_events(subscriber_id, channel_id, action, created_at)
			VALUES ($1, $2, 'subscribe', $3)`,
			s.subscriberID, s.channelID, created); err != nil {
			return fmt.Errorf("seed sub events: %w", err)
		}
	}

	// ---------------- Ads (две демонстрационные рекламы) ----------------
	type adSeed struct {
		title, description, videoURL string
		active                       bool
	}
	ads := []adSeed{
		{"Big Buck Bunny — короткий ролик",
			"Пример рекламного видео (Blender Foundation).",
			"https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4",
			true},
		{"Elephants Dream — превью",
			"Ещё один открытый ролик для демонстрации.",
			"https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ElephantsDream.mp4",
			false},
	}
	for _, ad := range ads {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ads(title, description, video_url, active)
			VALUES ($1, $2, $3, $4)`,
			ad.title, ad.description, ad.videoURL, ad.active); err != nil {
			return fmt.Errorf("seed ads: %w", err)
		}
	}

	if err := seedBulkChannels(ctx, tx, now); err != nil {
		return fmt.Errorf("seed bulk channels: %w", err)
	}

	return tx.Commit(ctx)
}
