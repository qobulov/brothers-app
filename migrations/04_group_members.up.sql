DO $schema$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
        CREATE TYPE user_role AS ENUM ('owner', 'admin', 'member', 'manager', 'employee', 'investor');
    ELSE
        IF NOT EXISTS (SELECT 1 FROM pg_enum WHERE enumtypid = 'user_role'::regtype AND enumlabel = 'manager') THEN
            ALTER TYPE user_role ADD VALUE 'manager';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_enum WHERE enumtypid = 'user_role'::regtype AND enumlabel = 'employee') THEN
            ALTER TYPE user_role ADD VALUE 'employee';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_enum WHERE enumtypid = 'user_role'::regtype AND enumlabel = 'investor') THEN
            ALTER TYPE user_role ADD VALUE 'investor';
        END IF;
    END IF;
END
$schema$;

CREATE TABLE IF NOT EXISTS group_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES groups(id),
    user_id uuid NOT NULL REFERENCES users(id),
    username varchar NOT NULL,
    role user_role NOT NULL,
    is_owner boolean NOT NULL DEFAULT false,
    joined_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT group_members_group_id_id_key UNIQUE (group_id, id)
);

DO $schema$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'group_members'::regclass
          AND conname = 'group_members_group_id_id_key'
    ) THEN
        ALTER TABLE group_members
            ADD CONSTRAINT group_members_group_id_id_key UNIQUE (group_id, id);
    END IF;
END
$schema$;

CREATE UNIQUE INDEX IF NOT EXISTS group_members_active_group_user_unique_idx
    ON group_members (group_id, user_id)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS group_members_one_active_owner_idx
    ON group_members (group_id)
    WHERE is_owner AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS group_members_user_idx ON group_members (user_id);
