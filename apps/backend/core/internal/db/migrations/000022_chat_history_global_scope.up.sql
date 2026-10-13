ALTER TABLE public.chat_conversations DROP COLUMN channel_id;

DELETE FROM public.chat_conversations t WHERE NOT EXISTS (SELECT 1 FROM public.admin_users a WHERE a.id = t.admin_id);

ALTER TABLE ONLY public.chat_conversations
    ADD CONSTRAINT chat_conversations_admin_id_fkey FOREIGN KEY (admin_id) REFERENCES public.admin_users(id) ON DELETE CASCADE;
