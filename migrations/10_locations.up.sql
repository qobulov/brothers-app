CREATE TABLE IF NOT EXISTS locations (
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

CREATE UNIQUE INDEX IF NOT EXISTS locations_active_group_name_idx
    ON locations (group_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS locations_active_group_employee_idx
    ON locations (group_id, employee_id)
    WHERE employee_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS locations_group_idx
    ON locations (group_id)
    WHERE deleted_at IS NULL;
