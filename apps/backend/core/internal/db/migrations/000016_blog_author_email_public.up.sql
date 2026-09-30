-- An author's email reaches the storefront only when this is set - it defaults off, so nobody's address goes public without a choice
ALTER TABLE public.blog_authors ADD COLUMN email_public boolean DEFAULT false NOT NULL;
