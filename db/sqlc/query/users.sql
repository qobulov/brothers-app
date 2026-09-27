-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT *
FROM users
WHERE deleted_at IS NULL
ORDER BY created_at;

-- name: SearchUsers :many
SELECT id, username, email, avatar_url
FROM users
WHERE deleted_at IS NULL
  AND is_active
  AND (
    lower(COALESCE(username, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
    OR lower(COALESCE(email, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
  )
ORDER BY username NULLS LAST, email NULLS LAST
LIMIT 20;

-- name: UpdateUserName :one
UPDATE users
SET name = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteUser :one
UPDATE users
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;
