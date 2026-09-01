-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: GetChirps :many
SELECT *
FROM chirps
WHERE ($1::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR user_id = $1)
ORDER BY
    CASE WHEN $2::text = 'DESC' THEN created_at END DESC,
    CASE WHEN $2::text = 'ASC' THEN created_at END ASC;;

-- name: GetChirpByID :one
SELECT *
FROM chirps
WHERE id = $1
ORDER BY created_at ASC;

-- name: DeleteChirp :exec
DELETE FROM chirps
WHERE id = $1 AND user_id = $2;