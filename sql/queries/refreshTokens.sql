-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (token, created_at, updated_at, expires_at, user_id)
VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3
)
RETURNING *;

-- name: GetRefreshTokenByTok :one
SELECT *
FROM refresh_tokens
WHERE token = $1;

-- name: UpdateRevokeAt :exec
UPDATE refresh_tokens
SET updated_at = NOW(), revoked_at = Now()
WHERE token = $1;