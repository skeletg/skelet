-- migrate:up
-- The suite forwards nothing now, so its job has no handler and would sit parked in the job list for good - its run
-- history goes with it
DELETE FROM scheduled_jobs WHERE name = 'event-destinations-forward' AND kind = 'code';

-- migrate:down
-- Nothing to put back: the scheduler seeds a row for every job the code registers when it boots
