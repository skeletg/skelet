-- Synapse's one commerce-side read - the customer universe a segment definition draws from

-- name: ChannelCustomerIDs :many
-- Every customer id in a channel - the universe a behavioral segment draws from
-- Used to evaluate negated / OR'd nodes in a segment definition tree (NOT X =
-- universe minus X)
SELECT id FROM customers WHERE channel_id = sqlc.arg(channel_id);
