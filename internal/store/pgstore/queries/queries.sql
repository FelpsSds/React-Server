
-- name: GetRoom :one
SELECT
    id, theme
FROM rooms
WHERE id = $1;

-- name: CreateRoom :many
SELECT
    id, theme
FROM rooms;

-- name: InsertRoom :one
INSERT INTO rooms (theme)
VALUES ($1)
RETURNING id;

-- name: GetMessage :many
SELECT
    id, room_id, message, reaction_count, answered
FROM messages
WHERE room_id = $1;

-- name: ReactMessage :one
UPDATE messages
SET reaction_count = reaction_count + 1
WHERE id = $1
RETURNING reaction_count;

-- name: RemoveReactionFromMessage :one
UPDATE messages
SET reaction_count = reaction_count - 1
WHERE id = $1
RETURNING reaction_count;

-- name: MarkMessageAsAnswered :exec
UPDATE messages
SET answered = true
WHERE id = $1;
