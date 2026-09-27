-- Group and invitation API compatibility layer. Existing group rows remain
-- valid while new memberships use manager, employee, and investor roles.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumtypid = 'user_role'::regtype AND enumlabel = 'manager'
    ) THEN
        ALTER TYPE user_role ADD VALUE 'manager';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumtypid = 'user_role'::regtype AND enumlabel = 'employee'
    ) THEN
        ALTER TYPE user_role ADD VALUE 'employee';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumtypid = 'user_role'::regtype AND enumlabel = 'investor'
    ) THEN
        ALTER TYPE user_role ADD VALUE 'investor';
    END IF;
END $$;

ALTER TABLE group_members
    ALTER COLUMN deleted_at DROP NOT NULL;

DROP INDEX IF EXISTS group_members_group_user_unique_idx;
CREATE UNIQUE INDEX IF NOT EXISTS group_members_active_group_user_unique_idx
    ON group_members (group_id, user_id)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS group_members_one_active_owner_idx
    ON group_members (group_id)
    WHERE is_owner AND deleted_at IS NULL;

ALTER TABLE group_invitations
    ALTER COLUMN phone DROP NOT NULL,
    ALTER COLUMN token_hash DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS invited_user_id uuid REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS email varchar(255),
    ADD COLUMN IF NOT EXISTS status varchar(16) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS responded_at timestamptz,
    ADD COLUMN IF NOT EXISTS revoked_at timestamptz;

ALTER TABLE group_invitations
    DROP CONSTRAINT IF EXISTS group_invitations_status_check;
ALTER TABLE group_invitations
    ADD CONSTRAINT group_invitations_status_check
    CHECK (status IN ('pending', 'accepted', 'rejected', 'revoked'));

UPDATE group_invitations invitation
SET invited_user_id = users.id,
    email = users.email
FROM users
WHERE invitation.invited_user_id IS NULL
  AND invitation.phone IS NOT NULL
  AND users.phone = invitation.phone
  AND users.deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS group_invitations_recipient_idx
    ON group_invitations (invited_user_id, status, expires_at)
    WHERE invited_user_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS group_invitations_one_pending_recipient_idx
    ON group_invitations (group_id, invited_user_id)
    WHERE status = 'pending' AND invited_user_id IS NOT NULL;
