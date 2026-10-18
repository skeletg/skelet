-- name: InsertProductQuestion :one
INSERT INTO product_questions (product_id, channel_id, customer_id, author_name, author_email, question)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListPublishedQuestions :many
SELECT * FROM product_questions
WHERE  product_id = $1 AND channel_id = $2 AND status = 'published'
ORDER  BY answered_at DESC NULLS LAST, created_at DESC
LIMIT  $3;

-- name: ListQuestionsForModeration :many
SELECT * FROM product_questions
WHERE  (sqlc.arg(all_channels)::bool OR channel_id = ANY (sqlc.arg(channel_ids)::uuid[]))
  AND  (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER  BY created_at DESC
LIMIT  $1 OFFSET $2;

-- name: GetProductQuestion :one
SELECT * FROM product_questions WHERE id = $1;

-- name: AnswerProductQuestion :one
-- prev_status is read under the row lock, so of two racing answers only one sees the question unpublished and announces it
WITH prev AS (
  SELECT p.id, p.status FROM product_questions p WHERE p.id = $1 FOR UPDATE
)
UPDATE product_questions q
SET    answer = $2, answered_by = $3, answered_by_integration_id = $4, answered_at = now(), status = 'published'
FROM   prev
WHERE  q.id = prev.id
RETURNING sqlc.embed(q), prev.status AS prev_status;

-- name: SetProductQuestionStatus :one
UPDATE product_questions
SET    status = $2
WHERE  id = $1
RETURNING *;

-- name: DeleteProductQuestion :exec
DELETE FROM product_questions WHERE id = $1;
