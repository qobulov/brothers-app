-- name: GetUserByIdentifier :one
SELECT id, phone, username, email, password, password_hash, first_name, last_name, avatar_url,
       language, is_active, last_login_at, created_at, updated_at, deleted_at
FROM users
WHERE (username = $1 OR phone = $2 OR email = $3) AND deleted_at IS NULL
FOR UPDATE;

-- name: GetActiveUser :one
SELECT id, phone, username, email, password, password_hash, first_name, last_name, avatar_url,
       language, is_active, last_login_at, created_at, updated_at, deleted_at
FROM users
WHERE id = $1 AND is_active = true AND deleted_at IS NULL;

-- name: RevokeActiveSession :exec
UPDATE user_sessions
SET revoked_at = $2
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: GetSessionByRefreshHash :one
SELECT id, user_id, refresh_token_hash, device_id, device_name, expires_at, created_at, revoked_at
FROM user_sessions
WHERE refresh_token_hash = $1 AND revoked_at IS NULL AND expires_at > $2
FOR UPDATE;

-- name: RotateSessionRefresh :one
UPDATE user_sessions
SET refresh_token_hash = $2, expires_at = $3
WHERE id = $1 AND refresh_token_hash = $4 AND revoked_at IS NULL AND expires_at > $5
RETURNING id, user_id, refresh_token_hash, device_id, device_name, expires_at, created_at, revoked_at;
