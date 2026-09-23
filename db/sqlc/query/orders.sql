-- name: CreateOrder :one
INSERT INTO orders (total)
VALUES (sqlc.arg(total)::float8)
RETURNING id, total::float8 AS total;

-- name: GetOrderByID :one
SELECT id, total::float8 AS total
FROM orders
WHERE id = $1;

-- name: ListOrders :many
SELECT id, total::float8 AS total
FROM orders
ORDER BY id;

-- name: UpdateOrder :one
UPDATE orders
SET total = sqlc.arg(total)::float8
WHERE id = sqlc.arg(id)
RETURNING id, total::float8 AS total;

-- name: DeleteOrder :one
DELETE FROM orders
WHERE id = $1
RETURNING id;
