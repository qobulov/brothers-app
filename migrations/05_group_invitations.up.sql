CREATE TABLE IF NOT EXISTS group_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES groups(id),
    invited_by uuid NOT NULL REFERENCES users(id),
    invited_user_id uuid REFERENCES users(id),
    location_name varchar(255),
    email varchar(255),
    phone varchar(20),
    role user_role NOT NULL,
    token_hash text UNIQUE,
    status varchar(16) NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    rejected_at timestamptz,
    responded_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT group_invitations_status_check
        CHECK (status IN ('pending', 'accepted', 'rejected', 'revoked'))
);

CREATE INDEX IF NOT EXISTS group_invitations_group_idx ON group_invitations (group_id);
CREATE INDEX IF NOT EXISTS group_invitations_phone_idx ON group_invitations (phone);
CREATE INDEX IF NOT EXISTS group_invitations_recipient_idx
    ON group_invitations (invited_user_id, status, expires_at)
    WHERE invited_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS group_invitations_one_pending_recipient_idx
    ON group_invitations (group_id, invited_user_id)
    WHERE status = 'pending' AND invited_user_id IS NOT NULL AND deleted_at IS NULL;
