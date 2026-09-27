-- One marker per (webhook, outbox event), so a replayed event can't enqueue a second delivery
CREATE TABLE public.webhook_enqueue_dedup (
    webhook_id uuid NOT NULL REFERENCES public.webhooks(id) ON DELETE CASCADE,
    dedupe_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (webhook_id, dedupe_key)
);

CREATE INDEX idx_webhook_enqueue_dedup_created ON public.webhook_enqueue_dedup USING btree (created_at);
