CREATE TABLE users (
    id uuid PRIMARY KEY,
    phone varchar(20),
    username varchar(50),
    email text,
    password text,
    password_hash text,
    first_name varchar(100),
    last_name varchar(100),
    avatar_url text,
    language varchar(10) NOT NULL,
    is_active boolean NOT NULL,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz
);

CREATE TABLE orders (
    id bigserial PRIMARY KEY,
    total numeric NOT NULL
);

CREATE TABLE user_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    refresh_token_hash text NOT NULL,
    device_id varchar(255),
    device_name varchar(255),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE TYPE user_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE groups (
    id uuid PRIMARY KEY,
    name varchar(255) NOT NULL,
    created_by uuid NOT NULL,
    is_active boolean NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz
);

CREATE TABLE group_members (
    id uuid PRIMARY KEY,
    group_id uuid NOT NULL,
    user_id uuid NOT NULL,
    username varchar NOT NULL,
    role user_role NOT NULL,
    is_owner boolean NOT NULL,
    joined_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted_at timestamptz
);

CREATE TABLE group_invitations (
    id uuid PRIMARY KEY,
    group_id uuid NOT NULL,
    invited_by uuid NOT NULL,
    phone varchar(20) NOT NULL,
    role user_role NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    rejected_at timestamptz,
    created_at timestamptz NOT NULL
);
