- **Catalogues can be delivered automatically to third parties.** Provider plugins
  share channel-scoped destination accounts, encrypted credentials, product snapshots
  and a durable retry queue. OpenAI receives compressed discovery feeds over verified
  SFTP; custom integrations receive signed JSONL webhooks. Each provider owns its
  format and upload logic, while large catalogue deliveries run separately from
  conversion events. Signals is optional.
