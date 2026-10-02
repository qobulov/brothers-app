-- name: GetUserByEmailOrUsername :one
SELECT * FROM users
WHERE (email = sqlc.arg(email) OR lower(username) = lower(sqlc.arg(username))) AND deleted_at IS NULL
FOR UPDATE;

-- name: CreateAuthUser :one
INSERT INTO users (
    id, password_hash, name, email, phone, username, first_name, last_name,
    avatar_url, language, is_active, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true, $11, $11)
RETURNING *;

-- name: GetUserByLogin :one
SELECT * FROM users
WHERE (lower(username) = lower(sqlc.arg(username)) OR email = sqlc.arg(email)) AND is_active = true AND deleted_at IS NULL;

-- name: UpdateUserLogin :one
UPDATE users SET last_login_at = $2, updated_at = $2
WHERE id = $1 AND is_active = true AND deleted_at IS NULL
RETURNING *;

-- name: GetActiveUser :one
SELECT * FROM users WHERE id = $1 AND is_active = true AND deleted_at IS NULL;

-- name: UpdateUserProfile :one
UPDATE users SET
    first_name = COALESCE(sqlc.narg('first_name'), first_name),
    last_name = COALESCE(sqlc.narg('last_name'), last_name),
    avatar_url = COALESCE(sqlc.narg('avatar_url'), avatar_url),
    language = COALESCE(sqlc.narg('language'), language),
    name = BTRIM(CONCAT_WS(' ',
        COALESCE(sqlc.narg('first_name'), first_name),
        COALESCE(sqlc.narg('last_name'), last_name)
    )),
    updated_at = sqlc.arg('updated_at')
WHERE id = sqlc.arg('id') AND is_active = true AND deleted_at IS NULL
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateUserPassword :execrows
UPDATE users SET password_hash = $2, updated_at = $3 WHERE id = $1 AND deleted_at IS NULL;

-- name: UsernameTaken :one
SELECT EXISTS (
    SELECT 1 FROM users WHERE lower(username) = lower(sqlc.arg(username)) AND deleted_at IS NULL
);
