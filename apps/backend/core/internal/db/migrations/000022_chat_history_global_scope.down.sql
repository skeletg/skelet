ALTER TABLE ONLY public.chat_conversations DROP CONSTRAINT IF EXISTS chat_conversations_admin_id_fkey;

ALTER TABLE public.chat_conversations ADD COLUMN channel_id uuid;

UPDATE public.chat_conversations
SET channel_id = (SELECT id FROM public.channels ORDER BY is_default DESC, created_at LIMIT 1);

DELETE FROM public.chat_conversations WHERE channel_id IS NULL;

ALTER TABLE public.chat_conversations
    ADD CONSTRAINT skeletor_conversations_channel_id_not_null NOT NULL channel_id;

ALTER TABLE ONLY public.chat_conversations
    ADD CONSTRAINT chat_conversations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
