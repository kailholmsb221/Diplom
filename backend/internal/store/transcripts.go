package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"videohub/internal/models"
)

// ---- Уровень видео: общий статус транскрипции и автоглавы ----

// SetVideoTranscriptStatus обновляет общий статус и (опционально) ошибку.
func (s *Store) SetVideoTranscriptStatus(ctx context.Context, videoID, status, errText string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE videos SET transcript_status=$2, transcript_error=$3 WHERE id=$1`,
		videoID, status, errText)
	return err
}

// SetVideoChapters сохраняет автогенерированные таймкоды (главы).
func (s *Store) SetVideoChapters(ctx context.Context, videoID string, chapters []models.Chapter) error {
	if chapters == nil {
		chapters = []models.Chapter{}
	}
	raw, err := json.Marshal(chapters)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx,
		`UPDATE videos SET chapters=$2 WHERE id=$1`, videoID, raw)
	return err
}

// SetVideoOriginalLanguage записывает определённый Whisper'ом язык.
func (s *Store) SetVideoOriginalLanguage(ctx context.Context, videoID, language string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE videos SET original_language=$2 WHERE id=$1`, videoID, language)
	return err
}

// ---- Уровень transcripts: одна строка на язык ----

const transcriptColumns = `video_id, language, is_original, status, full_text, segments, vtt_path, error, updated_at`

func scanTranscript(row interface{ Scan(...any) error }, t *models.Transcript) error {
	var segmentsRaw []byte
	err := row.Scan(&t.VideoID, &t.Language, &t.IsOriginal, &t.Status, &t.FullText,
		&segmentsRaw, &t.VTTUrl, &t.Error, &t.UpdatedAt)
	if err != nil {
		return err
	}
	if len(segmentsRaw) > 0 {
		_ = json.Unmarshal(segmentsRaw, &t.Segments)
	}
	if t.Segments == nil {
		t.Segments = []models.TranscriptSegment{}
	}
	return nil
}

func (s *Store) GetTranscript(ctx context.Context, videoID, language string) (*models.Transcript, error) {
	var t models.Transcript
	err := scanTranscript(s.Pool.QueryRow(ctx,
		`SELECT `+transcriptColumns+` FROM transcripts WHERE video_id=$1 AND language=$2`,
		videoID, language), &t)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) ListTranscriptsForVideo(ctx context.Context, videoID string) ([]models.Transcript, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+transcriptColumns+` FROM transcripts WHERE video_id=$1 ORDER BY language`, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Transcript, 0)
	for rows.Next() {
		var t models.Transcript
		if err := scanTranscript(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpsertTranscript создаёт/обновляет транскрипт для языка.
// Все поля кроме video_id+language опциональны: если nil — поле не трогается.
type UpsertTranscriptParams struct {
	VideoID    string
	Language   string
	IsOriginal *bool
	Status     *string
	FullText   *string
	Segments   *[]models.TranscriptSegment
	VTTPath    *string
	Error      *string
}

func (s *Store) UpsertTranscript(ctx context.Context, p UpsertTranscriptParams) error {
	var segmentsRaw any
	if p.Segments != nil {
		raw, err := json.Marshal(*p.Segments)
		if err != nil {
			return err
		}
		segmentsRaw = raw
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO transcripts(video_id, language, is_original, status, full_text, segments, vtt_path, error, updated_at)
		VALUES ($1, $2, COALESCE($3, FALSE), COALESCE($4, 'NOT_STARTED'),
		        COALESCE($5, ''), COALESCE($6, '[]'::jsonb), COALESCE($7, ''), COALESCE($8, ''), NOW())
		ON CONFLICT (video_id, language) DO UPDATE SET
			is_original = COALESCE($3, transcripts.is_original),
			status      = COALESCE($4, transcripts.status),
			full_text   = COALESCE($5, transcripts.full_text),
			segments    = COALESCE($6, transcripts.segments),
			vtt_path    = COALESCE($7, transcripts.vtt_path),
			error       = COALESCE($8, transcripts.error),
			updated_at  = NOW()`,
		p.VideoID, p.Language, p.IsOriginal, p.Status,
		p.FullText, segmentsRaw, p.VTTPath, p.Error)
	return err
}
