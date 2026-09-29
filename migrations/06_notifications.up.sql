DO $schema$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'notification_type') THEN
        CREATE TYPE notification_type AS ENUM ('GLOBAL', 'TARGETED');
    END IF;
END
$schema$;

CREATE TABLE IF NOT EXISTS notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title_uz varchar(255) NOT NULL,
    title_ru varchar(255) NOT NULL,
    title_en varchar(255) NOT NULL,
    content_uz text NOT NULL,
    content_ru text NOT NULL,
    content_en text NOT NULL,
    type notification_type NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
