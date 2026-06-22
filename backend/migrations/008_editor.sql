-- =====================================================================
-- Встроенный видеоредактор: чанковая загрузка, серверный рендер (экспорт),
-- ручные субтитры (SRT) и кадры-кандидаты для обложки.
--
-- Адаптировано под текущую инфраструктуру: вместо S3 — локальные файлы
-- (uploads/), вместо Celery — Python-worker render.py, запускаемый спавнером.
-- =====================================================================

-- Сессия чанковой (multipart) загрузки. Части складываются на диск в
-- uploads/TMP/<id>/<n>.part, по complete склеиваются в uploads/MP4/<uuid>.<ext>.
CREATE TABLE IF NOT EXISTS upload_sessions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name      TEXT NOT NULL,
    ext            TEXT NOT NULL,                       -- ".mp4" / ".webm"
    content_type   TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes     BIGINT NOT NULL DEFAULT 0,           -- заявленный размер
    total_parts    INT    NOT NULL DEFAULT 1,
    received_parts INT    NOT NULL DEFAULT 0,
    received_bytes BIGINT NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'open'
                   CHECK (status IN ('open','completed','aborted')),
    final_path     TEXT NOT NULL DEFAULT '',            -- путь относительно uploads, напр. MP4/<uuid>.mp4
    final_url      TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_upload_sessions_user ON upload_sessions(user_id, created_at DESC);

-- Задача рендера (экспорта) по EditManifest. Прогресс пишет render.py (шаг 1%).
CREATE TABLE IF NOT EXISTS render_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id    UUID NULL REFERENCES videos(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'queued'
                CHECK (status IN ('queued','processing','completed','failed')),
    progress    INT  NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    manifest    JSONB NOT NULL,                         -- EditManifest целиком
    output_url  TEXT NOT NULL DEFAULT '',               -- ссылка на HLS master.m3u8
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_render_jobs_video ON render_jobs(video_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_render_jobs_status ON render_jobs(status);

-- Ручные субтитры (SRT) — отдельно от авто-расшифровки (transcripts).
-- Один кортеж = язык. Отдаются как .vtt через конвертацию на лету.
CREATE TABLE IF NOT EXISTS video_subtitles (
    video_id    UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    language    TEXT NOT NULL,
    srt_content TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (video_id, language)
);

-- Кадры-кандидаты для обложки (FFmpeg каждые 10% длительности).
CREATE TABLE IF NOT EXISTS video_thumbnails (
    video_id      UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    frame_percent INT  NOT NULL CHECK (frame_percent BETWEEN 0 AND 100),
    url           TEXT NOT NULL,                         -- ссылка на кадр в uploads/PNG
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (video_id, frame_percent)
);
