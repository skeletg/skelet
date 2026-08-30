package admin

type storedMsg struct { // shape of the JSONB array in chat_conversations.messages - lives here so gqlgen can't evict it from chat.resolvers.go
	Role    string `json:"role"`
	Content string `json:"content"`
}
