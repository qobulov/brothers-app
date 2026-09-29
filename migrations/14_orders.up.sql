-- Two-employee orders: each party confirms the amount that changed hands.

-- The 03_orders stub stored only a total. Those rows cannot be converted, so
-- they are removed once; the guard keeps re-runs from deleting real orders.
DO $schema$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'orders'
          AND column_name = 'total'
    ) THEN
        DELETE FROM orders;
    END IF;
END
$schema$;

ALTER TABLE orders DROP COLUMN IF EXISTS total;

ALTER TABLE orders
    ALTER COLUMN group_id SET NOT NULL,
    ADD COLUMN IF NOT EXISTS created_by uuid NOT NULL REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS giver_member_id uuid NOT NULL,
    ADD COLUMN IF NOT EXISTS receiver_member_id uuid NOT NULL,
    ADD COLUMN IF NOT EXISTS giver_location_id uuid REFERENCES locations(id),
    ADD COLUMN IF NOT EXISTS receiver_location_id uuid REFERENCES locations(id),
    ADD COLUMN IF NOT EXISTS giver_customer_id uuid NOT NULL REFERENCES customers(id),
    ADD COLUMN IF NOT EXISTS receiver_customer_id uuid NOT NULL REFERENCES customers(id),
    ADD COLUMN IF NOT EXISTS amount_usd bigint NOT NULL,
    ADD COLUMN IF NOT EXISTS fee_uzs bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS status varchar(16) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS completed_at timestamptz,
    ADD COLUMN IF NOT EXISTS cancelled_at timestamptz;

DO $schema$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'orders'::regclass AND conname = 'orders_giver_member_fkey') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_giver_member_fkey
            FOREIGN KEY (group_id, giver_member_id) REFERENCES group_members(group_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'orders'::regclass AND conname = 'orders_receiver_member_fkey') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_receiver_member_fkey
            FOREIGN KEY (group_id, receiver_member_id) REFERENCES group_members(group_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'orders'::regclass AND conname = 'orders_distinct_parties_check') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_distinct_parties_check
            CHECK (giver_member_id <> receiver_member_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'orders'::regclass AND conname = 'orders_amount_usd_check') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_amount_usd_check CHECK (amount_usd > 0);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'orders'::regclass AND conname = 'orders_fee_uzs_check') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_fee_uzs_check CHECK (fee_uzs >= 0);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'orders'::regclass AND conname = 'orders_status_check') THEN
        ALTER TABLE orders ADD CONSTRAINT orders_status_check
            CHECK (status IN ('pending', 'completed', 'cancelled'));
    END IF;
END
$schema$;

CREATE INDEX IF NOT EXISTS orders_giver_idx
    ON orders (giver_member_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS orders_receiver_idx
    ON orders (receiver_member_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS order_confirmations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES orders(id),
    member_id uuid NOT NULL REFERENCES group_members(id),
    amount_usd bigint NOT NULL CHECK (amount_usd > 0),
    fee_uzs bigint NOT NULL DEFAULT 0 CHECK (fee_uzs >= 0),
    confirmed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS order_confirmations_active_member_idx
    ON order_confirmations (order_id, member_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS order_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES orders(id),
    actor_user_id uuid NOT NULL REFERENCES users(id),
    event_type varchar(50) NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS order_events_order_created_idx
    ON order_events (order_id, created_at, id)
    WHERE deleted_at IS NULL;
