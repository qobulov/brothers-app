CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique_idx
    ON users (username)
    WHERE username IS NOT NULL AND deleted_at IS NULL;
DROP INDEX IF EXISTS users_username_lower_unique_idx;
