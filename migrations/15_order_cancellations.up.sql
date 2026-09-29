-- A cancellation needs both parties: the requester approves by asking, the
-- other party approves or rejects.
CREATE TABLE IF NOT EXISTS order_cancellations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES orders(id),
    requested_by_member_id uuid NOT NULL REFERENCES group_members(id),
    reason varchar(500),
    status varchar(16) NOT NULL DEFAULT 'pending',
    responded_by_member_id uuid REFERENCES group_members(id),
    responded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT order_cancellations_status_check
        CHECK (status IN ('pending', 'approved', 'rejected'))
);

CREATE UNIQUE INDEX IF NOT EXISTS order_cancellations_one_pending_idx
    ON order_cancellations (order_id)
    WHERE status = 'pending' AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS order_cancellations_order_created_idx
    ON order_cancellations (order_id, created_at DESC)
    WHERE deleted_at IS NULL;
