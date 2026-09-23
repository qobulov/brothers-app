-- name: GetRegistrationUserForUpdate :one
SELECT * FROM users
WHERE (phone = $1 OR username = $2) AND deleted_at IS NULL
FOR UPDATE;

-- name: CreatePendingUser :one
INSERT INTO users (
    id, email, password_hash, name, phone, username, first_name, last_name,
    avatar_url, language, is_active, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, false, $11, $11)
RETURNING *;

-- name: UpdatePendingUser :one
UPDATE users SET
    email = $2, password_hash = $3, name = $4, phone = $5, username = $6,
    first_name = $7, last_name = $8, avatar_url = $9, language = $10,
    is_active = false, updated_at = $11
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ActivateUser :one
UPDATE users SET is_active = true, updated_at = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: GetUserByIdentifier :one
SELECT * FROM users
WHERE (username = $1 OR phone = $2 OR email = $3) AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateUserLogin :one
UPDATE users SET last_login_at = $2, updated_at = $2 WHERE id = $1 RETURNING *;

-- name: GetActiveUser :one
SELECT * FROM users WHERE id = $1 AND is_active = true AND deleted_at IS NULL;

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
