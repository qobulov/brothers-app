CREATE TABLE IF NOT EXISTS employee_balances (
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

DO $schema$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'employee_balances'
          AND column_name = 'balance_usd_cents'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'employee_balances'
          AND column_name = 'balance_usd'
    ) THEN
        ALTER TABLE employee_balances RENAME COLUMN balance_usd_cents TO balance_usd;
    END IF;
END
$schema$;

CREATE UNIQUE INDEX IF NOT EXISTS employee_balances_active_group_member_idx
    ON employee_balances (group_id, member_id)
    WHERE deleted_at IS NULL;

INSERT INTO employee_balances (group_id, member_id)
SELECT members.group_id, members.id
FROM group_members members
WHERE members.role::text = 'employee'
  AND members.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM employee_balances balances
      WHERE balances.group_id = members.group_id
        AND balances.member_id = members.id
        AND balances.deleted_at IS NULL
  );
