-- Settings saves were audited with the plaintext value of secret-typed settings, since "path" and "value" don't look secret - redact the rows already written for core's secret settings
UPDATE public.audit_log
SET detail = jsonb_set(detail, '{value}', '"[REDACTED]"')
WHERE detail ? 'value'
  AND detail->>'path' IN ('ai/llm/api_key', 'cms/forms/captcha_secret_key')
  AND detail->>'value' IS DISTINCT FROM '[REDACTED]';
