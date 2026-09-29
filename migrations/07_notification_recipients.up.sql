CREATE TABLE IF NOT EXISTS notification_recipients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id uuid NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_read boolean NOT NULL DEFAULT false,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (notification_id, user_id)
);

CREATE INDEX IF NOT EXISTS notification_recipients_unread_idx
    ON notification_recipients (user_id, is_read, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
