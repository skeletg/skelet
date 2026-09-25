UPDATE scheduled_jobs SET timeout_seconds = 300
WHERE name IN ('trait-compute', 'segment-refresh', 'synapse-segment-refresh') AND timeout_seconds = 1800;
