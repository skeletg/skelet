-- name: InsertReferralCode :one
INSERT INTO referral_codes (customer_id, code)
VALUES ($1, $2)
ON CONFLICT (customer_id) DO UPDATE SET customer_id = EXCLUDED.customer_id
RETURNING *;

-- name: GetReferralCodeByCustomer :one
SELECT * FROM referral_codes WHERE customer_id = $1;

-- name: GetReferralCodeForReferee :one
-- A code only refers someone on its owner's own channel, since each channel runs its own program
SELECT rc.* FROM referral_codes rc
JOIN customers owner ON owner.id = rc.customer_id
JOIN customers referee ON referee.id = sqlc.arg(referee_id)::uuid
WHERE rc.code = sqlc.arg(code) AND owner.channel_id = referee.channel_id;

-- name: InsertReferral :execrows
-- One referral per referee, ever - re-registration attempts and code
-- swaps are silently ignored via the unique constraint
INSERT INTO referrals (referrer_customer_id, referee_customer_id, code)
VALUES ($1, $2, $3)
ON CONFLICT (referee_customer_id) DO NOTHING;

-- name: GetPendingReferralByReferee :one
-- Locked read inside the reward tx: the status flip below is the
-- idempotency guard for at-least-once OrderPlaced delivery
SELECT * FROM referrals
WHERE  referee_customer_id = $1 AND status = 'pending'
FOR UPDATE;

-- name: MarkReferralRewarded :exec
UPDATE referrals
SET    status = 'rewarded', order_id = $2,
       referrer_reward_cents = $3, referee_reward_cents = $4,
       rewarded_at = now()
WHERE  id = $1;

-- name: ReferralStats :one
SELECT COUNT(*)::BIGINT                                                   AS referred,
       COUNT(*) FILTER (WHERE status = 'rewarded')::BIGINT                AS rewarded,
       COALESCE(SUM(referrer_reward_cents), 0)::BIGINT                    AS earned_cents
FROM   referrals
WHERE  referrer_customer_id = $1;

-- name: GetRewardedReferralByOrder :one
-- Locked read for the cancellation reversal - the status flip back to 'reversed' is what makes the reversal idempotent under at-least-once OrderCancelled delivery
SELECT * FROM referrals
WHERE  order_id = $1 AND status = 'rewarded'
FOR UPDATE;

-- name: MarkReferralReversed :exec
-- Terminal state: a reversed referral is never re-rewarded, so a cancelled
-- first order can't be replayed into another payout
UPDATE referrals
SET    status = 'reversed', rewarded_at = NULL
WHERE  id = $1;
