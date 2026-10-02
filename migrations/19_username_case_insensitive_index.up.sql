-- Usernames keep their typed case but are unique regardless of case, so
-- Abror and abror cannot both exist. If such a pair already exists this fails
-- loudly instead of silently merging accounts.
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_unique_idx
    ON users (lower(username))
    WHERE username IS NOT NULL AND deleted_at IS NULL;

-- The case-insensitive index also covers exact matches.
DROP INDEX IF EXISTS users_username_unique_idx;
