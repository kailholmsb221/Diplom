package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"videohub/internal/models"
)

// ---------------------- Сессии чанковой загрузки ----------------------

const uploadSessionColumns = `id, user_id, file_name, ext, content_type, size_bytes,
	total_parts, received_parts, received_bytes, status, final_path, final_url, created_at, updated_at`

func scanUploadSession(row rowScanner, s *models.UploadSession) error {
	return row.Scan(&s.ID, &s.UserID, &s.FileName, &s.Ext, &s.ContentType, &s.SizeBytes,
		&s.TotalParts, &s.ReceivedParts, &s.ReceivedBytes, &s.Status, &s.FinalPath, &s.FinalURL,
		&s.CreatedAt, &s.UpdatedAt)
}

// CreateUploadSession создаёт сессию загрузки и возвращает её с заполненными id/датами.
func (s *Store) CreateUploadSession(ctx context.Context, in models.UploadSession) (*models.UploadSession, error) {
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	if in.TotalParts < 1 {
		in.TotalParts = 1
	}
	if in.ContentType == "" {
		in.ContentType = "application/octet-stream"
	}
	var out models.UploadSession
	err := scanUploadSession(s.Pool.QueryRow(ctx, `
		INSERT INTO upload_sessions (id, user_id, file_name, ext, content_type, size_bytes, total_parts)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING `+uploadSessionColumns,
		in.ID, in.UserID, in.FileName, in.Ext, in.ContentType, in.SizeBytes, in.TotalParts), &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) GetUploadSession(ctx context.Context, id string) (*models.UploadSession, error) {
	var out models.UploadSession
	err := scanUploadSession(s.Pool.QueryRow(ctx,
		`SELECT `+uploadSessionColumns+` FROM upload_sessions WHERE id=$1`, id), &out)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AddUploadSessionPart атомарно учитывает принятую часть (счётчики частей и байт).
func (s *Store) AddUploadSessionPart(ctx context.Context, id string, partBytes int64) (*models.UploadSession, error) {
	var out models.UploadSession
	err := scanUploadSession(s.Pool.QueryRow(ctx, `
		UPDATE upload_sessions
		SET received_parts = received_parts + 1,
		    received_bytes = received_bytes + $2,
		    updated_at     = NOW()
		WHERE id=$1 AND status='open'
		RETURNING `+uploadSessionColumns, id, partBytes), &out)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CompleteUploadSession помечает сессию завершённой и фиксирует итоговый файл.
func (s *Store) CompleteUploadSession(ctx context.Context, id, finalPath, finalURL string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE upload_sessions
		SET status='completed', final_path=$2, final_url=$3, updated_at=NOW()
		WHERE id=$1`, id, finalPath, finalURL)
	return err
}

// ---------------------- Задачи рендера (экспорта) ----------------------

const renderJobColumns = `id, video_id, user_id, status, progress, manifest, output_url, error, created_at, updated_at`

func scanRenderJob(row rowScanner, j *models.RenderJob) error {
	var manifestRaw []byte
	if err := row.Scan(&j.ID, &j.VideoID, &j.UserID, &j.Status, &j.Progress,
		&manifestRaw, &j.OutputURL, &j.Error, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return err
	}
	if len(manifestRaw) > 0 {
		_ = json.Unmarshal(manifestRaw, &j.Manifest)
	}
	if j.Manifest.Clips == nil {
		j.Manifest.Clips = []models.EditClip{}
	}
	return nil
}

// CreateRenderJob ставит задачу рендера в очередь (status=queued, progress=0).
func (s *Store) CreateRenderJob(ctx context.Context, videoID *string, userID string, m models.EditManifest) (*models.RenderJob, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out models.RenderJob
	err = scanRenderJob(s.Pool.QueryRow(ctx, `
		INSERT INTO render_jobs (id, video_id, user_id, manifest)
		VALUES ($1,$2,$3,$4)
		RETURNING `+renderJobColumns,
		uuid.NewString(), videoID, userID, raw), &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) GetRenderJob(ctx context.Context, id string) (*models.RenderJob, error) {
	var out models.RenderJob
	err := scanRenderJob(s.Pool.QueryRow(ctx,
		`SELECT `+renderJobColumns+` FROM render_jobs WHERE id=$1`, id), &out)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetRenderJobProgress обновляет прогресс (0..100) и переводит в processing.
func (s *Store) SetRenderJobProgress(ctx context.Context, id string, progress int) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE render_jobs
		SET progress=$2, status='processing', updated_at=NOW()
		WHERE id=$1 AND status IN ('queued','processing')`, id, progress)
	return err
}

// SetRenderJobDone фиксирует финальный статус: completed (с outputURL) или failed (с errText).
func (s *Store) SetRenderJobDone(ctx context.Context, id, status, outputURL, errText string) error {
	progress := 0
	if status == models.RenderCompleted {
		progress = 100
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE render_jobs
		SET status=$2,
		    progress = CASE WHEN $2='completed' THEN 100 ELSE progress END,
		    output_url=$3, error=$4, updated_at=NOW()
		WHERE id=$1`, id, status, outputURL, errText)
	_ = progress
	return err
}

// ---------------------- Проекты редактора (черновики) ----------------------

const editorProjectColumns = `id, user_id, title, source_video_id, is_remix, timeline,
	status, last_render_job_id, result_video_id, created_at, updated_at`

func scanEditorProject(row rowScanner, p *models.EditorProject) error {
	var timelineRaw []byte
	if err := row.Scan(&p.ID, &p.UserID, &p.Title, &p.SourceVideoID, &p.IsRemix, &timelineRaw,
		&p.Status, &p.LastRenderJobID, &p.ResultVideoID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return err
	}
	if len(timelineRaw) == 0 {
		timelineRaw = []byte("{}")
	}
	p.Timeline = json.RawMessage(timelineRaw)
	return nil
}

// CreateEditorProject создаёт черновик монтажа.
func (s *Store) CreateEditorProject(ctx context.Context, p models.EditorProject) (*models.EditorProject, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if len(p.Timeline) == 0 {
		p.Timeline = json.RawMessage("{}")
	}
	if p.Title == "" {
		p.Title = "Без названия"
	}
	var out models.EditorProject
	err := scanEditorProject(s.Pool.QueryRow(ctx, `
		INSERT INTO editor_projects (id, user_id, title, source_video_id, is_remix, timeline)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING `+editorProjectColumns,
		p.ID, p.UserID, p.Title, p.SourceVideoID, p.IsRemix, []byte(p.Timeline)), &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetEditorProject возвращает проект по id (без проверки владельца — её делает хендлер).
func (s *Store) GetEditorProject(ctx context.Context, id string) (*models.EditorProject, error) {
	var out models.EditorProject
	err := scanEditorProject(s.Pool.QueryRow(ctx,
		`SELECT `+editorProjectColumns+` FROM editor_projects WHERE id=$1`, id), &out)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListEditorProjects возвращает проекты пользователя (свежие сверху).
func (s *Store) ListEditorProjects(ctx context.Context, userID string) ([]models.EditorProject, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+editorProjectColumns+` FROM editor_projects WHERE user_id=$1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.EditorProject, 0)
	for rows.Next() {
		var p models.EditorProject
		if err := scanEditorProject(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateEditorProject сохраняет title и/или timeline (автосейв и ручное сохранение).
func (s *Store) UpdateEditorProject(ctx context.Context, id string, title *string, timeline json.RawMessage) (*models.EditorProject, error) {
	var timelineArg any
	if timeline != nil {
		timelineArg = []byte(timeline)
	}
	var out models.EditorProject
	err := scanEditorProject(s.Pool.QueryRow(ctx, `
		UPDATE editor_projects
		SET title    = COALESCE($2, title),
		    timeline = COALESCE($3, timeline),
		    updated_at = NOW()
		WHERE id=$1
		RETURNING `+editorProjectColumns, id, title, timelineArg), &out)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetEditorProjectExport привязывает к проекту задачу рендера и видео-результат,
// переводя его в статус exporting.
func (s *Store) SetEditorProjectExport(ctx context.Context, id, jobID, resultVideoID string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE editor_projects
		SET status='exporting', last_render_job_id=$2, result_video_id=$3, updated_at=NOW()
		WHERE id=$1`, id, jobID, resultVideoID)
	return err
}

// MarkEditorProjectExported переводит проект в exported (вызывается после успешного рендера).
func (s *Store) MarkEditorProjectExported(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE editor_projects SET status='exported', updated_at=NOW() WHERE id=$1`, id)
	return err
}

// DeleteEditorProject удаляет проект.
func (s *Store) DeleteEditorProject(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM editor_projects WHERE id=$1`, id)
	return err
}

// AuthorizeClipSource проверяет, что пользователь вправе использовать файл url как клип:
//   - это его собственное видео (канал принадлежит userID);
//   - либо чужое видео с allow_remix=TRUE (легальный ремикс);
//   - либо файл, который этот пользователь сам загрузил в редакторе (upload_sessions).
//
// Источник всегда проверяется на уровне хендлера ещё и localFileFromURL (анти-SSRF):
// сюда попадают только ссылки на /uploads/*, внешние URL отсекаются раньше.
func (s *Store) AuthorizeClipSource(ctx context.Context, userID, url string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM videos v JOIN channels c ON c.id=v.channel_id
				   WHERE v.video_url=$2 AND (c.owner_id=$1 OR v.allow_remix=TRUE))
			OR EXISTS(SELECT 1 FROM upload_sessions
				   WHERE user_id=$1 AND final_url=$2 AND status='completed')`,
		userID, url).Scan(&ok)
	return ok, err
}
