-- Manual balance corrections made by managers. Rows are never updated or
-- deleted; a wrong adjustment is corrected with another adjustment.
CREATE TABLE IF NOT EXISTS balance_adjustments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL,
    member_id uuid NOT NULL,
    old_balance_usd bigint NOT NULL,
    new_balance_usd bigint NOT NULL,
    amount_usd bigint NOT NULL,
    reason varchar(500),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT balance_adjustments_group_member_fkey
        FOREIGN KEY (group_id, member_id) REFERENCES group_members(group_id, id),
    CONSTRAINT balance_adjustments_amount_check
        CHECK (amount_usd = new_balance_usd - old_balance_usd AND amount_usd <> 0)
);

CREATE INDEX IF NOT EXISTS balance_adjustments_member_created_idx
    ON balance_adjustments (group_id, member_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
