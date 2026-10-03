ALTER TABLE public.channels ADD COLUMN storefront_host text DEFAULT ''::text NOT NULL;

-- Mirrors channel.storefrontHost: lowercase host, userinfo and path dropped, :80 dropped unless https, :443 dropped unless http
WITH parsed AS (
    SELECT id,
           coalesce(substring(lower(btrim(public_url)) FROM '^([a-z][a-z0-9+.-]*)://'), '') AS scheme,
           regexp_replace(
               substring(regexp_replace(lower(btrim(public_url)), '^([a-z][a-z0-9+.-]*:)?//', '') FROM '^[^/?#]*'),
               '^.*@', '') AS hostport
    FROM public.channels
    WHERE btrim(public_url) <> ''
)
UPDATE public.channels c
SET storefront_host = CASE
        WHEN p.hostport LIKE '%:80' AND p.scheme <> 'https' THEN left(p.hostport, -3)
        WHEN p.hostport LIKE '%:443' AND p.scheme <> 'http' THEN left(p.hostport, -4)
        ELSE rtrim(p.hostport, ':')
    END
FROM parsed p
WHERE p.id = c.id;

-- Colliding hosts: the oldest channel (created_at, id) keeps its host, the rest are blanked and stop matching by Host until their public_url is fixed
UPDATE public.channels c
SET storefront_host = ''
FROM (
    SELECT id, row_number() OVER (PARTITION BY storefront_host ORDER BY created_at, id) AS rn
    FROM public.channels
    WHERE storefront_host <> ''
) d
WHERE d.id = c.id AND d.rn > 1;

DROP INDEX IF EXISTS public.idx_channels_public_url;
CREATE UNIQUE INDEX idx_channels_storefront_host ON public.channels USING btree (storefront_host) WHERE (storefront_host <> ''::text);
