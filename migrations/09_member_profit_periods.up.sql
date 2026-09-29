CREATE TABLE IF NOT EXISTS member_profit_periods (
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

DO $schema$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'member_profit_periods'
          AND column_name = 'profit_uzs_tiyin'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'member_profit_periods'
          AND column_name = 'profit_uzs'
    ) THEN
        ALTER TABLE member_profit_periods RENAME COLUMN profit_uzs_tiyin TO profit_uzs;
    END IF;
END
$schema$;

CREATE UNIQUE INDEX IF NOT EXISTS member_profit_periods_active_period_idx
    ON member_profit_periods (group_id, member_id, year, month)
    WHERE deleted_at IS NULL;
