-- Reverse insert order, so a row is never deleted while something still points at it
DELETE FROM public.shipping_carriers;
DELETE FROM public.review_moderation_tags;
DELETE FROM public.review_categories;
DELETE FROM public.regions;
DELETE FROM public.email_groups;
DELETE FROM public.channel_locales;
DELETE FROM public.customer_groups;
DELETE FROM public.admin_role_permission_scopes;
DELETE FROM public.permissions;
DELETE FROM public.permission_groups;
DELETE FROM public.channels;
DELETE FROM public.locales;
DELETE FROM public.admin_roles;
DELETE FROM public.countries;
DELETE FROM public.currencies;
