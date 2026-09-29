DROP TABLE IF EXISTS order_events;
DROP TABLE IF EXISTS order_confirmations;

ALTER TABLE orders
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS fee_uzs,
    DROP COLUMN IF EXISTS amount_usd,
    DROP COLUMN IF EXISTS receiver_customer_id,
    DROP COLUMN IF EXISTS giver_customer_id,
    DROP COLUMN IF EXISTS receiver_location_id,
    DROP COLUMN IF EXISTS giver_location_id,
    DROP COLUMN IF EXISTS receiver_member_id,
    DROP COLUMN IF EXISTS giver_member_id,
    DROP COLUMN IF EXISTS created_by,
    ALTER COLUMN group_id DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS total numeric NOT NULL DEFAULT 0;
