-- name: InsertProductQuestion :one
INSERT INTO product_questions (product_id, channel_id, customer_id, author_name, author_email, question)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListPublishedQuestions :many
SELECT * FROM product_questions
WHERE  product_id = $1 AND status = 'published'
ORDER  BY answered_at DESC NULLS LAST, created_at DESC
LIMIT  $2;

-- name: ListQuestionsForModeration :many
SELECT * FROM product_questions
WHERE  (sqlc.arg(all_channels)::bool OR channel_id = ANY (sqlc.arg(channel_ids)::uuid[]))
  AND  (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER  BY created_at DESC
LIMIT  $1 OFFSET $2;

-- name: GetProductQuestion :one
SELECT * FROM product_questions WHERE id = $1;

-- name: AnswerProductQuestion :one
UPDATE product_questions
SET    answer = $2, answered_by = $3, answered_at = now(), status = 'published'
WHERE  id = $1
RETURNING *;

-- name: SetProductQuestionStatus :one
UPDATE product_questions
SET    status = $2
WHERE  id = $1
RETURNING *;

-- name: DeleteProductQuestion :exec
DELETE FROM product_questions WHERE id = $1;
