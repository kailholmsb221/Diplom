-- =====================================================================
-- Встроенный видеоредактор: проекты монтажа (черновики) + ремикс-атрибуция.
--
-- editor_projects хранит НЕразрушающую раскладку монтажа (timeline) как JSONB.
-- Оригинальные файлы видео никогда не меняются — экспорт рождает НОВОЕ видео.
--
-- Поля ремикса на videos:
--   allow_remix       — автор разрешил «Создать свою версию» (opt-in);
--   source_video_id   — это видео получено монтажом/ремиксом из другого (атрибуция);
--   source_channel_id — канал-первоисточник (для подписи «Оригинал: …»).
-- =====================================================================

ALTER TABLE videos ADD COLUMN IF NOT EXISTS allow_remix       BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS source_video_id   UUID NULL REFERENCES videos(id)   ON DELETE SET NULL;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS source_channel_id UUID NULL REFERENCES channels(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_videos_source ON videos(source_video_id);

-- Проект редактора (черновик монтажа). Один пользователь → много проектов.
CREATE TABLE IF NOT EXISTS editor_projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    title           TEXT NOT NULL DEFAULT 'Без названия',
    -- Видео-первоисточник проекта: своё (монтаж) или чужое разрешённое (ремикс).
    source_video_id UUID NULL REFERENCES videos(id) ON DELETE SET NULL,
    is_remix        BOOLEAN NOT NULL DEFAULT FALSE,
    -- Полное состояние раскладки (дорожки, клипы, трим, цвет, звук). См. timeline JSON.
    timeline        JSONB NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','exporting','exported')),
    -- Последняя задача рендера (для отображения прогресса и анти-дабла экспорта).
    last_render_job_id UUID NULL REFERENCES render_jobs(id) ON DELETE SET NULL,
    -- Видео, рождённое из проекта после экспорта (NULL пока не экспортировали).
    result_video_id UUID NULL REFERENCES videos(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_editor_projects_user ON editor_projects(user_id, updated_at DESC);
