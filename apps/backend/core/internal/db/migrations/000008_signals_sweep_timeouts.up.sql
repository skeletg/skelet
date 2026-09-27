-- The suite's channel-walking sweeps get the maintenance call's 30 minutes, unless an operator already chose a timeout
UPDATE scheduled_jobs SET timeout_seconds = 1800
WHERE name IN ('trait-compute', 'segment-refresh', 'synapse-segment-refresh') AND timeout_seconds = 300;
