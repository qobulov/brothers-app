CREATE TABLE IF NOT EXISTS customers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES groups(id),
    phone varchar(20) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS customers_active_group_phone_idx
    ON customers (group_id, phone)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS customers_group_idx
    ON customers (group_id)
    WHERE deleted_at IS NULL;
