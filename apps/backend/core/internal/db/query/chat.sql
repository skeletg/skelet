-- name: ListChatConversations :many
SELECT id, title, updated_at
FROM chat_conversations
WHERE admin_id = $1
ORDER BY updated_at DESC
LIMIT $2;

-- name: GetChatConversation :one
SELECT id, title, messages, updated_at
FROM chat_conversations
WHERE id = $1 AND admin_id = $2;

-- name: InsertChatConversation :one
INSERT INTO chat_conversations (admin_id, title, messages)
VALUES ($1, $2, $3)
RETURNING id;

-- name: UpdateChatConversation :one
UPDATE chat_conversations
SET title = $3, messages = $4, updated_at = now()
WHERE id = $1 AND admin_id = $2
RETURNING id;

-- name: DeleteChatConversation :exec
DELETE FROM chat_conversations WHERE id = $1 AND admin_id = $2;

-- name: DeleteChatConversationsBefore :execrows
DELETE FROM chat_conversations WHERE updated_at < $1;
