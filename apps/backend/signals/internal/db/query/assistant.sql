-- name: RecordAssistantTurn :execrows
WITH conv AS (
    INSERT INTO assistant_conversations AS c (id, channel_id, customer_id, anon_id, last_at)
    VALUES (sqlc.arg(conversation_id), sqlc.arg(channel_id), sqlc.narg(customer_id), sqlc.arg(anon_id), sqlc.arg(at))
    ON CONFLICT (id) DO UPDATE SET last_at = GREATEST(c.last_at, EXCLUDED.last_at), customer_id = COALESCE(c.customer_id, EXCLUDED.customer_id)
    WHERE c.channel_id = EXCLUDED.channel_id
    RETURNING c.id
)
INSERT INTO assistant_turns (
    id, conversation_id, customer_id, page, question, reply, recap, follow_ups, rounds, products, cart_changed, checkout, incomplete,
    found_nothing, unsure, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, at
)
SELECT sqlc.arg(id), conv.id, sqlc.narg(customer_id), sqlc.arg(page), sqlc.arg(question), sqlc.arg(reply), sqlc.arg(recap),
    sqlc.arg(follow_ups)::text[], sqlc.arg(rounds)::jsonb, sqlc.arg(products), sqlc.arg(cart_changed), sqlc.arg(checkout), sqlc.arg(incomplete),
    sqlc.arg(found_nothing), sqlc.arg(unsure), sqlc.arg(model), sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(cache_read_tokens), sqlc.arg(cache_write_tokens), sqlc.arg(at)
FROM conv
ON CONFLICT (id) DO NOTHING;

-- The cursor is always set - the first page passes one past every row - so the row comparison stays an index condition.
-- Asking for a kind of unanswered message is a probe into the partial index per conversation walked, and OFFSET 0 keeps it
-- one - without it Postgres may hash every store's unanswered turns instead
-- name: ListAssistantConversations :many
SELECT s.id, s.customer_id, s.anon_id, s.first_question, s.started_at, s.last_at, s.turns, s.cart_changed, s.checkout, s.incomplete,
    COALESCE(g.unanswered, 0)::integer AS unanswered, COALESCE(g.id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS gap_id, COALESCE(g.question, '')::text AS gap_question,
    COALESCE(g.no_answer, false)::boolean AS gap_no_answer, COALESCE(g.cut_short, false)::boolean AS gap_cut_short,
    COALESCE(g.found_nothing, false)::boolean AS gap_found_nothing, COALESCE(g.unsure, false)::boolean AS gap_unsure
FROM assistant_conversation_summaries s
LEFT JOIN LATERAL (
    SELECT t.id, t.question, NOT t.incomplete AND t.reply = '' AND t.recap = '' AS no_answer, t.incomplete AS cut_short,
        t.found_nothing, t.unsure, count(*) OVER () AS unanswered
    FROM assistant_turns t
    WHERE t.conversation_id = s.id
      AND (t.incomplete OR t.found_nothing OR t.unsure OR (t.reply = '' AND t.recap = ''))
      AND CASE sqlc.arg(unanswered)::text
            WHEN 'no_answer' THEN NOT t.incomplete AND t.reply = '' AND t.recap = ''
            WHEN 'cut_short' THEN t.incomplete
            WHEN 'found_nothing' THEN t.found_nothing
            WHEN 'unsure' THEN t.unsure
            ELSE TRUE
          END
    ORDER BY t.at DESC, t.id DESC
    LIMIT 1
) g ON TRUE
WHERE s.channel_id = sqlc.arg(channel_id)
  AND (s.last_at, s.id) < (sqlc.arg(before_at)::timestamptz, sqlc.arg(before_id)::uuid)
  AND (sqlc.narg(customer_id)::uuid IS NULL OR s.customer_id = sqlc.narg(customer_id)::uuid)
  AND (sqlc.arg(unanswered)::text = '' OR EXISTS (
      SELECT 1 FROM assistant_turns t
      WHERE t.conversation_id = s.id
        AND (t.incomplete OR t.found_nothing OR t.unsure OR (t.reply = '' AND t.recap = ''))
        AND CASE sqlc.arg(unanswered)::text
              WHEN 'no_answer' THEN NOT t.incomplete AND t.reply = '' AND t.recap = ''
              WHEN 'cut_short' THEN t.incomplete
              WHEN 'found_nothing' THEN t.found_nothing
              WHEN 'unsure' THEN t.unsure
              ELSE TRUE
            END
      OFFSET 0))
ORDER BY s.last_at DESC, s.id DESC
LIMIT sqlc.arg(lim);

-- The list with a search - its own query, so the trigram match stays a top-level semi-join Postgres can start from
-- name: SearchAssistantConversations :many
SELECT s.id, s.customer_id, s.anon_id, s.first_question, s.started_at, s.last_at, s.turns, s.cart_changed, s.checkout, s.incomplete,
    COALESCE(g.unanswered, 0)::integer AS unanswered, COALESCE(g.id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS gap_id, COALESCE(g.question, '')::text AS gap_question,
    COALESCE(g.no_answer, false)::boolean AS gap_no_answer, COALESCE(g.cut_short, false)::boolean AS gap_cut_short,
    COALESCE(g.found_nothing, false)::boolean AS gap_found_nothing, COALESCE(g.unsure, false)::boolean AS gap_unsure
FROM assistant_conversation_summaries s
LEFT JOIN LATERAL (
    SELECT t.id, t.question, NOT t.incomplete AND t.reply = '' AND t.recap = '' AS no_answer, t.incomplete AS cut_short,
        t.found_nothing, t.unsure, count(*) OVER () AS unanswered
    FROM assistant_turns t
    WHERE t.conversation_id = s.id
      AND (t.incomplete OR t.found_nothing OR t.unsure OR (t.reply = '' AND t.recap = ''))
      AND CASE sqlc.arg(unanswered)::text
            WHEN 'no_answer' THEN NOT t.incomplete AND t.reply = '' AND t.recap = ''
            WHEN 'cut_short' THEN t.incomplete
            WHEN 'found_nothing' THEN t.found_nothing
            WHEN 'unsure' THEN t.unsure
            ELSE TRUE
          END
    ORDER BY t.at DESC, t.id DESC
    LIMIT 1
) g ON TRUE
WHERE s.channel_id = sqlc.arg(channel_id)
  AND (s.last_at, s.id) < (sqlc.arg(before_at)::timestamptz, sqlc.arg(before_id)::uuid)
  AND (sqlc.narg(customer_id)::uuid IS NULL OR s.customer_id = sqlc.narg(customer_id)::uuid)
  AND (sqlc.arg(unanswered)::text = '' OR EXISTS (
      SELECT 1 FROM assistant_turns t
      WHERE t.conversation_id = s.id
        AND (t.incomplete OR t.found_nothing OR t.unsure OR (t.reply = '' AND t.recap = ''))
        AND CASE sqlc.arg(unanswered)::text
              WHEN 'no_answer' THEN NOT t.incomplete AND t.reply = '' AND t.recap = ''
              WHEN 'cut_short' THEN t.incomplete
              WHEN 'found_nothing' THEN t.found_nothing
              WHEN 'unsure' THEN t.unsure
              ELSE TRUE
            END
      OFFSET 0))
  AND s.id IN (
      SELECT t.conversation_id FROM assistant_turns t
      JOIN assistant_conversations c ON c.id = t.conversation_id AND c.channel_id = sqlc.arg(channel_id)
      WHERE t.question ILIKE '%' || sqlc.arg(search)::text || '%' ESCAPE '\' OR t.reply ILIKE '%' || sqlc.arg(search)::text || '%' ESCAPE '\')
ORDER BY s.last_at DESC, s.id DESC
LIMIT sqlc.arg(lim);

-- name: GetAssistantConversation :one
SELECT s.id, s.customer_id, s.anon_id, s.first_question, s.started_at, s.last_at, s.turns, s.cart_changed, s.checkout, s.incomplete,
    COALESCE(g.unanswered, 0)::integer AS unanswered, COALESCE(g.id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS gap_id, COALESCE(g.question, '')::text AS gap_question,
    COALESCE(g.no_answer, false)::boolean AS gap_no_answer, COALESCE(g.cut_short, false)::boolean AS gap_cut_short,
    COALESCE(g.found_nothing, false)::boolean AS gap_found_nothing, COALESCE(g.unsure, false)::boolean AS gap_unsure
FROM assistant_conversation_summaries s
LEFT JOIN LATERAL (
    SELECT t.id, t.question, NOT t.incomplete AND t.reply = '' AND t.recap = '' AS no_answer, t.incomplete AS cut_short,
        t.found_nothing, t.unsure, count(*) OVER () AS unanswered
    FROM assistant_turns t
    WHERE t.conversation_id = s.id
      AND (t.incomplete OR t.found_nothing OR t.unsure OR (t.reply = '' AND t.recap = ''))
    ORDER BY t.at DESC, t.id DESC
    LIMIT 1
) g ON TRUE
WHERE s.id = sqlc.arg(conversation_id) AND s.channel_id = sqlc.arg(channel_id);

-- Newest first, so a cap keeps the latest - the rounds stay behind, summed up
-- name: ListAssistantTurns :many
SELECT t.id, t.conversation_id, t.customer_id, c.anon_id, t.page, t.question, t.reply, t.recap, t.follow_ups, t.products, t.cart_changed,
    t.checkout, t.incomplete, t.found_nothing, t.unsure, t.model, t.input_tokens, t.output_tokens, t.cache_read_tokens, t.cache_write_tokens, t.at,
    jsonb_array_length(t.rounds)::integer AS steps,
    (SELECT count(*) FROM jsonb_array_elements(t.rounds) r, jsonb_array_elements(COALESCE(r->'calls', '[]'::jsonb)) k)::integer AS tool_calls,
    jsonb_path_exists(t.rounds, '$[*].calls[*] ? (@.isError == true)')::boolean AS refused,
    jsonb_path_exists(t.rounds, '$[*].thinking')::boolean AS thought
FROM assistant_turns t
JOIN assistant_conversations c ON c.id = t.conversation_id
WHERE c.id = sqlc.arg(conversation_id) AND c.channel_id = sqlc.arg(channel_id)
ORDER BY t.at DESC, t.id DESC
LIMIT sqlc.arg(lim);

-- name: GetAssistantTurnRounds :one
SELECT t.rounds FROM assistant_turns t
JOIN assistant_conversations c ON c.id = t.conversation_id
WHERE t.id = sqlc.arg(turn_id) AND c.channel_id = sqlc.arg(channel_id);

-- Whole conversations that went quiet first, then old turns inside ones still going - in batches, so a long backlog makes progress
-- name: PruneAssistantConversations :execrows
DELETE FROM assistant_conversations
WHERE id IN (SELECT old.id FROM assistant_conversations old WHERE old.last_at < sqlc.arg(before) LIMIT sqlc.arg(lim));

-- name: PruneAssistantTurns :execrows
DELETE FROM assistant_turns
WHERE id IN (SELECT old.id FROM assistant_turns old WHERE old.at < sqlc.arg(before) LIMIT sqlc.arg(lim));

-- A customer's turns go wherever they sit - one who signed in on someone else's tab has turns in that conversation
-- name: EraseAssistantConversations :execrows
WITH turns AS (
    DELETE FROM assistant_turns WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[])
)
DELETE FROM assistant_conversations
WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]) OR (anon_id <> '' AND anon_id = ANY(sqlc.arg(anon_ids)::text[]));

-- Conversations with each kind of message the store couldn't answer - the condition is written as the partial index has it
-- name: CountAssistantConversationGaps :one
SELECT
    count(DISTINCT t.conversation_id)::integer AS any,
    count(DISTINCT t.conversation_id) FILTER (WHERE NOT t.incomplete AND t.reply = '' AND t.recap = '')::integer AS no_answer,
    count(DISTINCT t.conversation_id) FILTER (WHERE t.incomplete)::integer AS cut_short,
    count(DISTINCT t.conversation_id) FILTER (WHERE t.found_nothing)::integer AS found_nothing,
    count(DISTINCT t.conversation_id) FILTER (WHERE t.unsure)::integer AS unsure
FROM assistant_turns t
JOIN assistant_conversations c ON c.id = t.conversation_id AND c.channel_id = sqlc.arg(channel_id)
WHERE (t.incomplete OR t.found_nothing OR t.unsure OR (t.reply = '' AND t.recap = ''))
  AND (sqlc.narg(customer_id)::uuid IS NULL OR c.customer_id = sqlc.narg(customer_id)::uuid);
