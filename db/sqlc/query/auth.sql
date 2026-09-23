-- name: GetUserByPhoneOrUsername :one
SELECT * FROM users
WHERE (phone = $1 OR username = $2) AND deleted_at IS NULL
FOR UPDATE;

-- name: CreateAuthUser :one
INSERT INTO users (
    id, password_hash, name, phone, username, first_name, last_name,
    avatar_url, language, is_active, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, $10, $10)
RETURNING *;

-- name: GetUserByLogin :one
SELECT * FROM users
WHERE (username = $1 OR phone = $2) AND is_active = true AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateUserLogin :one
UPDATE users SET last_login_at = $2, updated_at = $2 WHERE id = $1 RETURNING *;

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

-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone = $1 AND deleted_at IS NULL;

-- name: UserPhoneExistsOther :one
SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1 AND id <> $2 AND deleted_at IS NULL);

-- name: UpdateUserPhone :execrows
UPDATE users SET phone = $2, updated_at = $3 WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserPassword :execrows
UPDATE users SET password_hash = $2, updated_at = $3 WHERE id = $1 AND deleted_at IS NULL;

-- name: RevokeActiveSession :exec
UPDATE user_sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL;

-- name: CreateSession :one
INSERT INTO user_sessions (id, user_id, refresh_token_hash, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetSessionByRefreshHash :one
SELECT * FROM user_sessions
WHERE refresh_token_hash = $1 AND revoked_at IS NULL AND expires_at > $2 FOR UPDATE;

-- name: RotateSessionRefresh :one
UPDATE user_sessions SET refresh_token_hash = $2, expires_at = $3
WHERE id = $1 AND refresh_token_hash = $4 AND revoked_at IS NULL AND expires_at > $5
RETURNING *;

-- name: RevokeSession :execrows
UPDATE user_sessions SET revoked_at = $3
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: GetActiveSession :one
SELECT * FROM user_sessions
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > $3;
