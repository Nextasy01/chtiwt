-- name: CreateChannel :one
INSERT INTO channels (user_id, name, stream_key_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetChannelByUserID :one
SELECT * FROM channels WHERE user_id = $1;

-- name: GetChannelByName :one
SELECT * FROM channels WHERE name = $1;

-- name: GetChannelByStreamKeyHash :one
SELECT * FROM channels WHERE stream_key_hash = $1;

-- name: UpdateChannelTitle :exec
UPDATE channels SET title = $2 WHERE id = $1;

-- name: UpdateChannelStreamKey :exec
UPDATE channels SET stream_key_hash = $2 WHERE id = $1;
