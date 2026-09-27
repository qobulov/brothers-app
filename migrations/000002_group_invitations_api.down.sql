DROP INDEX IF EXISTS group_invitations_one_pending_recipient_idx;
DROP INDEX IF EXISTS group_invitations_recipient_idx;
ALTER TABLE group_invitations DROP CONSTRAINT IF EXISTS group_invitations_status_check;
ALTER TABLE group_invitations
    DROP COLUMN IF EXISTS revoked_at,
    DROP COLUMN IF EXISTS responded_at,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS invited_user_id;
DROP INDEX IF EXISTS group_members_one_active_owner_idx;
DROP INDEX IF EXISTS group_members_active_group_user_unique_idx;
CREATE UNIQUE INDEX IF NOT EXISTS group_members_group_user_unique_idx ON group_members (group_id, user_id);
