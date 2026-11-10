- **Read the shopping assistant's conversations.** With `sales/agents/assistant_transcripts` on, each turn is
  kept in Signals with the shopper's message, the reply, and how the model got there: a summary of its thinking
  where the model gives one, the words that reached the shopper, and every tool it called with what the store
  answered. Only shoppers signed in or who agreed to analytics are kept. **Agentic Commerce, Conversations**
  lists them per store with a search, under the new `assistant.view` permission, and flags a message the shopper
  got no answer to. Messages age out after `signals/assistant/retention_days` (90) and go with a customer's
  erasure, guest chats included.
