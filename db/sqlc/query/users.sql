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
