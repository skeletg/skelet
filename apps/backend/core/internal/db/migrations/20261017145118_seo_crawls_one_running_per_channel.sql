-- migrate:up
CREATE UNIQUE INDEX seo_crawls_one_running_idx ON public.seo_crawls (channel_id) WHERE status = 'running';

-- migrate:down
DROP INDEX public.seo_crawls_one_running_idx;
