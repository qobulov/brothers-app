CREATE TABLE IF NOT EXISTS orders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid REFERENCES groups(id),
    total numeric NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS group_id uuid REFERENCES groups(id);

CREATE INDEX IF NOT EXISTS orders_group_created_idx
    ON orders (group_id, created_at DESC)
    WHERE group_id IS NOT NULL AND deleted_at IS NULL;
