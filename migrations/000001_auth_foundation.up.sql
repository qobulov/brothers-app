CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email varchar(255) UNIQUE,
    password text,
    password_hash text,
    name varchar(255),
    phone varchar(20),
    username varchar(50),
    first_name varchar(100),
    last_name varchar(100),
    avatar_url text,
    language varchar(10) NOT NULL DEFAULT 'uz',
    is_active boolean NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS users_phone_unique_idx ON users (phone) WHERE phone IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique_idx ON users (username) WHERE username IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS orders (
    id bigserial PRIMARY KEY,
    total numeric NOT NULL
);

CREATE TABLE IF NOT EXISTS user_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    refresh_token_hash text NOT NULL,
    device_id varchar(255),
    device_name varchar(255),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS user_sessions_refresh_token_hash_idx ON user_sessions (refresh_token_hash);
CREATE UNIQUE INDEX IF NOT EXISTS user_sessions_one_active_idx ON user_sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS user_sessions_expiry_idx ON user_sessions (expires_at);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
        CREATE TYPE user_role AS ENUM ('owner', 'admin', 'member');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(255) NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

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
    deleted_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS group_members_group_user_unique_idx ON group_members (group_id, user_id);
CREATE INDEX IF NOT EXISTS group_members_user_idx ON group_members (user_id);

CREATE TABLE IF NOT EXISTS group_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES groups(id),
    invited_by uuid NOT NULL REFERENCES users(id),
    phone varchar(20) NOT NULL,
    role user_role NOT NULL,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    rejected_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS group_invitations_group_idx ON group_invitations (group_id);
CREATE INDEX IF NOT EXISTS group_invitations_phone_idx ON group_invitations (phone);
