package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"videohub/internal/models"
)

// Все запросы видео отдают счётчики с уже применёнными админ-оверрайдами канала.
const videoSelect = `
SELECT
	v.id,
	v.channel_id,
	v.title,
	v.description,
	v.thumbnail_url,
	v.video_url,
	v.duration_sec,
	GREATEST(v.views_count    + COALESCE(o.views, 0), 0)    AS views,
	GREATEST(v.likes_count    + COALESCE(o.likes, 0), 0)    AS likes,
	GREATEST(v.dislikes_count + COALESCE(o.dislikes, 0), 0) AS dislikes,
	v.category,
	v.visibility,
	v.tags,
	v.uploaded_at,
	c.name        AS channel_name,
	c.avatar_url  AS channel_avatar,
	v.transcript_status,
	v.original_language,
	v.chapters,
	v.allow_remix,
	v.source_video_id,
	v.source_channel_id,
	sc.name AS source_channel_name
FROM videos v
JOIN channels c ON c.id = v.channel_id
LEFT JOIN channel_overrides o ON o.channel_id = v.channel_id
LEFT JOIN channels sc ON sc.id = v.source_channel_id
`

type ListVideosParams struct {
	Query      string
	ChannelID  string
	Limit      int
	OnlyPublic bool
	ExcludeID  string
	Category   string
	OrderBy    string // "rating", "fresh"; пусто = uploaded_at DESC
}

func (s *Store) ListVideos(ctx context.Context, p ListVideosParams) ([]models.Video, error) {
	var (
		args  []any
		conds []string
	)
	add := func(c string, a ...any) {
		conds = append(conds, c)
		args = append(args, a...)
	}
	if p.ChannelID != "" {
		add("v.channel_id = $"+itoa(len(args)+1), p.ChannelID)
	}
	if p.ExcludeID != "" {
		add("v.id <> $"+itoa(len(args)+1), p.ExcludeID)
	}
	if p.OnlyPublic {
		conds = append(conds, "v.visibility = 'public'")
	}
	if p.Category != "" {
		add("v.category = $"+itoa(len(args)+1), p.Category)
	}
	if p.Query != "" {
		q := "%" + strings.ToLower(p.Query) + "%"
		add(`(LOWER(v.title) LIKE $`+itoa(len(args)+1)+` OR LOWER(v.description) LIKE $`+itoa(len(args)+1)+`)`, q)
	}

	sql := videoSelect
	if len(conds) > 0 {
		sql += " WHERE " + strings.Join(conds, " AND ")
	}
	switch p.OrderBy {
	case "rating":
		// Формула из ТЗ + бонус свежести (см. recommend.go); тут только базовый порядок.
		sql += `
		ORDER BY
			GREATEST(v.views_count + COALESCE(o.views,0), 0) * 1
			+ GREATEST(v.likes_count + COALESCE(o.likes,0), 0) * 5
			- GREATEST(v.dislikes_count + COALESCE(o.dislikes,0), 0) * 3
			+ GREATEST(0, 14 - EXTRACT(DAY FROM (NOW() - v.uploaded_at))) * 50
			DESC, v.uploaded_at DESC`
	default:
		sql += " ORDER BY v.uploaded_at DESC"
	}
	if p.Limit > 0 {
		sql += " LIMIT $" + itoa(len(args)+1)
		args = append(args, p.Limit)
	}

	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Video, 0)
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetVideo(ctx context.Context, id string) (*models.Video, error) {
	row := s.Pool.QueryRow(ctx, videoSelect+" WHERE v.id=$1", id)
	v, err := scanVideoRow(row)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// CreateVideo вставляет запись и возвращает созданное видео.
func (s *Store) CreateVideo(ctx context.Context, in models.Video) (*models.Video, error) {
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO videos
			(id, channel_id, title, description, thumbnail_url, video_url, duration_sec,
			 category, visibility, tags, allow_remix, source_video_id, source_channel_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		in.ID, in.ChannelID, in.Title, in.Description, in.ThumbnailURL, in.VideoURL, in.DurationSec,
		in.Category, in.Visibility, in.Tags, in.AllowRemix, in.SourceVideoID, in.SourceChannelID)
	if err != nil {
		return nil, err
	}
	return s.GetVideo(ctx, in.ID)
}

// SetVideoAllowRemix переключает разрешение на ремикс. Только владелец канала видео
// (проверяется на уровне хендлера через ownerOfVideo).
func (s *Store) SetVideoAllowRemix(ctx context.Context, videoID string, allow bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE videos SET allow_remix=$2 WHERE id=$1`, videoID, allow)
	return err
}

// SetVideoOutput фиксирует результат серверного рендера: путь к файлу, обложку,
// длительность и видимость. Вызывается render.py по завершении экспорта.
func (s *Store) SetVideoOutput(ctx context.Context, videoID, videoURL, thumbnailURL string, durationSec int, visibility string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE videos
		SET video_url=$2,
		    thumbnail_url = CASE WHEN $3 <> '' THEN $3 ELSE thumbnail_url END,
		    duration_sec  = CASE WHEN $4 > 0 THEN $4 ELSE duration_sec END,
		    visibility    = $5
		WHERE id=$1`, videoID, videoURL, thumbnailURL, durationSec, visibility)
	return err
}

// OwnerOfVideo возвращает user_id владельца канала, которому принадлежит видео.
func (s *Store) OwnerOfVideo(ctx context.Context, videoID string) (string, error) {
	var ownerID string
	err := s.Pool.QueryRow(ctx,
		`SELECT c.owner_id FROM videos v JOIN channels c ON c.id=v.channel_id WHERE v.id=$1`,
		videoID).Scan(&ownerID)
	if isNoRows(err) {
		return "", ErrNotFound
	}
	return ownerID, err
}

func (s *Store) DeleteVideo(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM videos WHERE id=$1`, id)
	return err
}

// AdminAdjustVideoStats точечно меняет views/likes/dislikes конкретного видео
// и пишет событие в admin_stat_events с video_id (для аудита).
// Счётчики приводятся к >= 0.
func (s *Store) AdminAdjustVideoStats(ctx context.Context, videoID string, views, likes, dislikes int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var channelID string
	if err := tx.QueryRow(ctx, `SELECT channel_id FROM videos WHERE id=$1`, videoID).Scan(&channelID); err != nil {
		if isNoRows(err) {
			return ErrNotFound
		}
		return err
	}

	var appliedViews, appliedLikes, appliedDislikes int64
	if err := tx.QueryRow(ctx, `
		WITH current AS (
			SELECT views_count, likes_count, dislikes_count
			FROM videos
			WHERE id = $1
		),
		updated AS (
			UPDATE videos SET
				views_count    = GREATEST(views_count    + $2, 0),
				likes_count    = GREATEST(likes_count    + $3, 0),
				dislikes_count = GREATEST(dislikes_count + $4, 0)
			WHERE id = $1
			RETURNING views_count, likes_count, dislikes_count
		)
		SELECT
			u.views_count - c.views_count,
			u.likes_count - c.likes_count,
			u.dislikes_count - c.dislikes_count
		FROM updated u, current c`,
		videoID, views, likes, dislikes).Scan(&appliedViews, &appliedLikes, &appliedDislikes); err != nil {
		return err
	}

	for _, ev := range []struct {
		kind  string
		delta int64
	}{{"views", appliedViews}, {"likes", appliedLikes}, {"dislikes", appliedDislikes}} {
		if ev.delta == 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO admin_stat_events(channel_id, video_id, kind, delta) VALUES ($1, $2, $3, $4)`,
			channelID, videoID, ev.kind, ev.delta); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// RegisterView пишет уникальный просмотр: один аккаунт = +1 к видео, даже если
// пользователь несколько раз открывает страницу. Анонимные открытия счётчик
// не увеличивают — посчитать уникальность без авторизации нельзя.
//
// Возвращает true, если просмотр был засчитан (новый), и false, если уже был.
func (s *Store) RegisterView(ctx context.Context, videoID, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var already bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM video_views WHERE video_id = $1 AND user_id = $2)`,
		videoID, userID).Scan(&already); err != nil {
		return false, err
	}
	if already {
		return false, tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO video_views(video_id, user_id) VALUES ($1, $2)`,
		videoID, userID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE videos SET views_count = views_count + 1 WHERE id = $1`,
		videoID); err != nil {
		return false, err
	}
	// Монетизация: канал получает 100 тенге за каждую полную 1000 суммарных просмотров.
	if _, err := tx.Exec(ctx, `
		WITH video_channel AS (
			SELECT channel_id FROM videos WHERE id = $1
		),
		channel_views AS (
			SELECT COALESCE(SUM(v.views_count), 0) AS views
			FROM videos v
			JOIN video_channel vc ON vc.channel_id = v.channel_id
		),
		payout AS (
			SELECT CASE WHEN views % $2 = 0 THEN $3 ELSE 0 END AS amount
			FROM channel_views
		)
		UPDATE channels SET
			balance      = balance + payout.amount,
			total_earned = total_earned + payout.amount
		FROM video_channel, payout
		WHERE channels.id = video_channel.channel_id`,
		videoID, ViewPayoutViews, ViewPayoutTenge); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// -------- helpers ----------

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVideoRow(row rowScanner) (models.Video, error) {
	var v models.Video
	var chaptersRaw []byte
	err := row.Scan(&v.ID, &v.ChannelID, &v.Title, &v.Description, &v.ThumbnailURL, &v.VideoURL, &v.DurationSec,
		&v.Views, &v.Likes, &v.Dislikes, &v.Category, &v.Visibility, &v.Tags, &v.UploadedAt,
		&v.ChannelName, &v.ChannelAvatar,
		&v.TranscriptStatus, &v.OriginalLanguage, &chaptersRaw,
		&v.AllowRemix, &v.SourceVideoID, &v.SourceChannelID, &v.SourceChannelName)
	if err != nil {
		return v, err
	}
	if len(chaptersRaw) > 0 {
		if jerr := json.Unmarshal(chaptersRaw, &v.Chapters); jerr != nil {
			// Не валим запрос — просто оставляем пустые главы.
			v.Chapters = nil
		}
	}
	if v.Chapters == nil {
		v.Chapters = []models.Chapter{}
	}
	return v, err
}

func scanVideo(row rowScanner) (models.Video, error) {
	return scanVideoRow(row)
}
