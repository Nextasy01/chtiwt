-- name: CreateSession :one
INSERT INTO sessions (id, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSessionWithUser :one
SELECT
    s.id           AS session_id,
    s.expires_at   AS session_expires_at,
    u.id           AS user_id,
    u.username     AS user_username,
    u.email        AS user_email,
    u.password_hash AS user_password_hash,
    u.created_at   AS user_created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();
