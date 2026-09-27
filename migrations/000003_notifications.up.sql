DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'notification_type') THEN
        CREATE TYPE notification_type AS ENUM ('GLOBAL', 'TARGETED');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS notifications (
    id bigserial PRIMARY KEY,
    title varchar(255) NOT NULL,
    content text NOT NULL,
    type notification_type NOT NULL,
    action_url varchar(500),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz
);

CREATE TABLE IF NOT EXISTS notification_recipients (
    id bigserial PRIMARY KEY,
    notification_id bigint NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_read boolean NOT NULL DEFAULT false,
    read_at timestamptz,
    UNIQUE (notification_id, user_id)
);

CREATE INDEX IF NOT EXISTS notification_recipients_unread_idx
    ON notification_recipients (user_id, is_read, id DESC);
