-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: UpdateUserEmailPass :one
UPDATE users
SET email = $1, hashed_password = $2
WHERE id = $3
RETURNING *;

-- name: DeleteAllUser :exec
TRUNCATE TABLE users CASCADE;