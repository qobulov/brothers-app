-- Personal debts. A debt belongs to one user, is visible only to them and is
-- independent of orders and groups.
CREATE TABLE IF NOT EXISTS debts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id uuid NOT NULL REFERENCES users(id),
    direction varchar(16) NOT NULL,
    person_name varchar(100) NOT NULL,
    person_phone varchar(20),
    currency varchar(3) NOT NULL,
    original_amount bigint NOT NULL,
    remaining_amount bigint NOT NULL,
    status varchar(16) NOT NULL DEFAULT 'active',
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT debts_direction_check CHECK (direction IN ('they_owe_me', 'i_owe')),
    CONSTRAINT debts_currency_check CHECK (currency IN ('USD', 'UZS')),
    CONSTRAINT debts_status_check CHECK (status IN ('active', 'completed')),
    -- The last line of defence: no code path can push a debt below zero.
    CONSTRAINT debts_amounts_check CHECK (original_amount > 0 AND remaining_amount BETWEEN 0 AND original_amount)
);

-- An earlier draft of this migration linked debts to orders; drop that column
-- where the draft was already applied.
ALTER TABLE debts DROP COLUMN IF EXISTS source_order_id;

CREATE INDEX IF NOT EXISTS debts_owner_created_idx
    ON debts (owner_user_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

-- Repayments are never updated or deleted.
CREATE TABLE IF NOT EXISTS debt_repayments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    debt_id uuid NOT NULL REFERENCES debts(id),
    amount bigint NOT NULL CHECK (amount > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS debt_repayments_debt_created_idx
    ON debt_repayments (debt_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
