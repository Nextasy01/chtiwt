-- name: CreateStreamSession :one
INSERT INTO stream_sessions (channel_id, started_at)
VALUES ($1, $2)
RETURNING *;

-- name: EndStreamSession :exec
UPDATE stream_sessions
SET ended_at  = $2,
    end_reason = $3
WHERE id = $1 AND ended_at IS NULL;

-- name: MarkOpenStreamSessionsOrphaned :exec
UPDATE stream_sessions
SET ended_at  = now(),
    end_reason = 'orphan_swept'
WHERE ended_at IS NULL;
