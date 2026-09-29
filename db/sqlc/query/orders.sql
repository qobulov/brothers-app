-- name: CreateOrder :one
INSERT INTO orders (total)
VALUES (sqlc.arg(total)::float8)
RETURNING id, total::float8 AS total;

-- name: GetOrderByID :one
SELECT id, total::float8 AS total
FROM orders
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListOrders :many
SELECT id, total::float8 AS total
FROM orders
WHERE deleted_at IS NULL
ORDER BY created_at, id;

-- name: UpdateOrder :one
UPDATE orders
SET total = sqlc.arg(total)::float8,
    updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING id, total::float8 AS total;

-- name: DeleteOrder :one
UPDATE orders
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;
