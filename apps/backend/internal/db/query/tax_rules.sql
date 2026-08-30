-- name: CreateTaxRule :one
INSERT INTO tax_rules (
    channel_id, name, priority, exclusivity_group, sort_order, is_active
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateTaxRule :one
UPDATE tax_rules
SET name              = $2,
    priority          = $3,
    exclusivity_group = $4,
    sort_order        = $5,
    is_active         = $6,
    updated_at        = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteTaxRule :exec
DELETE FROM tax_rules WHERE id = $1;

-- name: GetTaxRule :one
SELECT * FROM tax_rules WHERE id = $1;

-- name: ListTaxRulesByChannel :many
SELECT * FROM tax_rules
WHERE channel_id = $1
ORDER BY priority, sort_order, name;

-- name: ListActiveTaxRulesByChannel :many
SELECT * FROM tax_rules
WHERE channel_id = $1 AND is_active = TRUE
ORDER BY priority, sort_order;

-- name: ListAllTaxRules :many
-- Admin cross-channel view (GLOBAL scope): every rule, regardless of
-- channel or active state
SELECT * FROM tax_rules
ORDER BY channel_id, priority, sort_order, name;

-- ==================== Conditions ====================

-- name: DeleteTaxRuleConditions :exec
DELETE FROM tax_rule_conditions WHERE rule_id = $1;

-- name: InsertTaxRuleCondition :one
INSERT INTO tax_rule_conditions (rule_id, type, params, params_version, sort_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListTaxRuleConditions :many
SELECT * FROM tax_rule_conditions
WHERE rule_id = $1
ORDER BY sort_order, id;

-- name: ListTaxRuleConditionsByRules :many
SELECT * FROM tax_rule_conditions
WHERE rule_id = ANY(sqlc.arg(rule_ids)::uuid[])
ORDER BY rule_id, sort_order, id;

-- ==================== Effects ====================

-- name: DeleteTaxRuleEffects :exec
DELETE FROM tax_rule_effects WHERE rule_id = $1;

-- name: InsertTaxRuleEffect :one
INSERT INTO tax_rule_effects (rule_id, type, params, params_version, sort_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListTaxRuleEffects :many
SELECT * FROM tax_rule_effects
WHERE rule_id = $1
ORDER BY sort_order, id;

-- name: ListTaxRuleEffectsByRules :many
SELECT * FROM tax_rule_effects
WHERE rule_id = ANY(sqlc.arg(rule_ids)::uuid[])
ORDER BY rule_id, sort_order, id;
