CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    phone varchar(20),
    username varchar(50),
    password text,
    password_hash text,
    first_name varchar(100),
    last_name varchar(100),
    avatar_url text,
    language varchar(10) NOT NULL,
    is_active boolean NOT NULL,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TYPE user_role AS ENUM ('owner', 'admin', 'member', 'manager', 'employee', 'investor');

CREATE TABLE groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(255) NOT NULL,
    created_by uuid NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TABLE orders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid REFERENCES groups(id),
    total numeric NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX orders_group_created_idx
    ON orders (group_id, created_at DESC)
    WHERE group_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE group_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL,
    user_id uuid NOT NULL,
    username varchar NOT NULL,
    role user_role NOT NULL,
    is_owner boolean NOT NULL DEFAULT false,
    joined_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT group_members_group_id_id_key UNIQUE (group_id, id)
);

CREATE TABLE group_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL,
    invited_by uuid NOT NULL,
    invited_user_id uuid,
    location_name varchar(255),
    email varchar(255),
    phone varchar(20),
    role user_role NOT NULL,
    token_hash text,
    status varchar(16) NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    rejected_at timestamptz,
    responded_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TYPE notification_type AS ENUM ('GLOBAL', 'TARGETED');

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title_uz varchar(255) NOT NULL,
    title_ru varchar(255) NOT NULL,
    title_en varchar(255) NOT NULL,
    content_uz text NOT NULL,
    content_ru text NOT NULL,
    content_en text NOT NULL,
    type notification_type NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    expires_at timestamptz
);

CREATE TABLE notification_recipients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id uuid NOT NULL,
    user_id uuid NOT NULL,
    is_read boolean NOT NULL DEFAULT false,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TABLE employee_balances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL,
    member_id uuid NOT NULL,
    balance_usd bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT employee_balances_group_member_fkey
        FOREIGN KEY (group_id, member_id)
        REFERENCES group_members(group_id, id)
        ON DELETE CASCADE
);

CREATE TABLE member_profit_periods (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL,
    member_id uuid NOT NULL,
    year smallint NOT NULL,
    month smallint NOT NULL,
    profit_uzs bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT member_profit_periods_group_member_fkey
        FOREIGN KEY (group_id, member_id)
        REFERENCES group_members(group_id, id)
        ON DELETE CASCADE,
    CONSTRAINT member_profit_periods_year_check CHECK (year BETWEEN 2000 AND 9999),
    CONSTRAINT member_profit_periods_month_check CHECK (month BETWEEN 1 AND 12)
);

CREATE TABLE locations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES groups(id),
    name varchar(255) NOT NULL,
    employee_id uuid,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT locations_group_employee_fkey
        FOREIGN KEY (group_id, employee_id)
        REFERENCES group_members(group_id, id)
);

CREATE UNIQUE INDEX locations_active_group_name_idx
    ON locations (group_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX locations_active_group_employee_idx
    ON locations (group_id, employee_id)
    WHERE employee_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX locations_group_idx
    ON locations (group_id)
    WHERE deleted_at IS NULL;

CREATE TABLE customers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES groups(id),
    phone varchar(20) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE UNIQUE INDEX customers_active_group_phone_idx
    ON customers (group_id, phone)
    WHERE deleted_at IS NULL;

CREATE INDEX customers_group_idx
    ON customers (group_id)
    WHERE deleted_at IS NULL;

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid REFERENCES groups(id),
    actor_user_id uuid NOT NULL REFERENCES users(id),
    action varchar(100) NOT NULL,
    entity_type varchar(50) NOT NULL,
    entity_id uuid NOT NULL,
    old_data jsonb,
    new_data jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX audit_logs_group_created_idx
    ON audit_logs (group_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX audit_logs_actor_created_idx
    ON audit_logs (actor_user_id, created_at DESC)
    WHERE deleted_at IS NULL;
