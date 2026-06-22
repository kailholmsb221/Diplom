-- =====================================================================
-- Транскрипция, перевод и таймкоды глав для видео.
-- Работает с Python-worker (см. backend/worker/transcribe.py).
-- =====================================================================

-- Сводный статус процесса транскрипции на уровне видео + автогенеренные главы.
ALTER TABLE videos ADD COLUMN IF NOT EXISTS transcript_status TEXT
    NOT NULL DEFAULT 'NOT_STARTED'
    CHECK (transcript_status IN ('NOT_STARTED','PROCESSING','TRANSLATING','COMPLETED','FAILED'));
ALTER TABLE videos ADD COLUMN IF NOT EXISTS original_language TEXT NULL
    CHECK (original_language IS NULL OR original_language IN ('ru','kk','en'));
ALTER TABLE videos ADD COLUMN IF NOT EXISTS chapters JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS transcript_error TEXT NOT NULL DEFAULT '';

-- Расшифровка / перевод на конкретный язык. Один кортеж = язык.
CREATE TABLE IF NOT EXISTS transcripts (
    video_id    UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    language    TEXT NOT NULL CHECK (language IN ('ru','kk','en')),
    is_original BOOLEAN NOT NULL DEFAULT FALSE,
    status      TEXT NOT NULL DEFAULT 'NOT_STARTED'
                CHECK (status IN ('NOT_STARTED','PROCESSING','TRANSLATING','COMPLETED','FAILED')),
    full_text   TEXT NOT NULL DEFAULT '',
    segments    JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- vtt_path относительно UPLOADS_DIR, например "SUBTITLES/<videoId>_ru.vtt".
    vtt_path    TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (video_id, language)
);

CREATE INDEX IF NOT EXISTS idx_transcripts_status ON transcripts(status);
