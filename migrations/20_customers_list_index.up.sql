-- The customer list is paged newest first; this index returns one page without
-- sorting every customer of the group.
CREATE INDEX IF NOT EXISTS customers_group_created_idx
    ON customers (group_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
