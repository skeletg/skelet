-- A token can name what it was minted for - the address a verification link went to - so redeeming it proves that address and no other
ALTER TABLE public.auth_tokens ADD COLUMN binding text DEFAULT '' NOT NULL;
