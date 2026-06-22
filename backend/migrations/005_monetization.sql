-- =====================================================================
-- Монетизация: реклама, премиум, баланс канала, транзакции.
-- Валюта — тенге, целые числа. 1000 просмотров = 100 тенге демо-заработка.
-- =====================================================================

-- ---------------- Премиум на пользователе ----------------
ALTER TABLE users ADD COLUMN IF NOT EXISTS premium       BOOLEAN     NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS premium_until TIMESTAMPTZ NULL;

-- ---------------- Баланс канала ----------------
-- balance       — доступно к выводу (накапливается из засчитанных просмотров,
--                 уменьшается при выводе, может быть скорректирован админом)
-- total_earned  — всего заработано за всё время (только просмотры + админские +)
ALTER TABLE channels ADD COLUMN IF NOT EXISTS balance      BIGINT NOT NULL DEFAULT 0;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS total_earned BIGINT NOT NULL DEFAULT 0;

-- ---------------- Рекламные ролики ----------------
CREATE TABLE IF NOT EXISTS ads (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    video_url   TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ads_active ON ads(active) WHERE active = TRUE;

-- ---------------- Транзакции ----------------
-- type:
--   PREMIUM_PURCHASE  — покупка пользователем премиума
--   CHANNEL_PAYOUT    — вывод денег с баланса канала
--   ADMIN_ADJUSTMENT  — ручная корректировка админом
CREATE TABLE IF NOT EXISTS transactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NULL REFERENCES users(id)    ON DELETE SET NULL,
    channel_id  UUID NULL REFERENCES channels(id) ON DELETE SET NULL,
    type        TEXT NOT NULL CHECK (type IN (
                    'PREMIUM_PURCHASE', 'CHANNEL_PAYOUT', 'ADMIN_ADJUSTMENT'
                )),
    amount      BIGINT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('SUCCESS', 'FAILED', 'PENDING')),
    description TEXT NOT NULL DEFAULT '',
    card_last4  TEXT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tx_user      ON transactions(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tx_channel   ON transactions(channel_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tx_type_date ON transactions(type, created_at DESC);
