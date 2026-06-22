-- Recalculate channel earnings from existing video views.
-- Rule: 100 tenge per each full 1000 views.
WITH earnings AS (
    SELECT
        c.id AS channel_id,
        FLOOR(COALESCE(SUM(v.views_count), 0)::numeric / 1000) * 100 AS earned
    FROM channels c
    LEFT JOIN videos v ON v.channel_id = c.id
    GROUP BY c.id
)
UPDATE channels c
SET
    balance = earnings.earned,
    total_earned = earnings.earned
FROM earnings
WHERE c.id = earnings.channel_id;
