-- One grant per order and file, so a replayed or racing order.placed can't mint a second token for the same purchase
-- Duplicates already minted collapse onto the earliest, which keeps the higher use count of the pair
UPDATE public.download_grants keep
SET    downloads_used = dup.used
FROM  (SELECT DISTINCT ON (order_id, download_id) id, max(downloads_used) OVER (PARTITION BY order_id, download_id) AS used
       FROM   public.download_grants WHERE order_id IS NOT NULL
       ORDER  BY order_id, download_id, created_at, id) dup
WHERE  keep.id = dup.id AND keep.downloads_used < dup.used;

DELETE FROM public.download_grants g
WHERE  g.order_id IS NOT NULL
  AND  EXISTS (SELECT 1 FROM public.download_grants e
               WHERE e.order_id = g.order_id AND e.download_id = g.download_id
                 AND (e.created_at, e.id) < (g.created_at, g.id));

CREATE UNIQUE INDEX download_grants_order_download_key ON public.download_grants USING btree (order_id, download_id) WHERE order_id IS NOT NULL;
