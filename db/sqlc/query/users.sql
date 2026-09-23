-- name: CreateUser :one
INSERT INTO users (id, email, password, name, language, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, email, password, name;

-- name: GetUserByEmail :one
SELECT id, email, password, name
FROM users
WHERE email = $1 AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT id, email, password, name
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT id, email, password, name
FROM users
WHERE deleted_at IS NULL
ORDER BY created_at;

-- name: UpdateUserName :one
UPDATE users
SET name = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, email, password, name;

-- name: SoftDeleteUser :one
UPDATE users
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;
