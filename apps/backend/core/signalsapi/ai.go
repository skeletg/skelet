package signalsapi

// What core and Signals agree on for the AI analytics - who did it, how much a call may carry, and how long after
// the assistant showed a product its cart or order still counts
const (
	AISourceAssistant = "assistant"
	AISourceAgent     = "agent"
	// an agent whose client gives no name
	AIUnknownAgent = "unknown"

	AIMaxTools     = 50
	AIMaxSearches  = 20
	AIMaxTerm      = 80 // runes of a search's words
	AIMaxAgentName = 64

	AICartWindowDays  = 7
	AIOrderWindowDays = 14

	// Who helped fill a cart, as orders carry it in assisted_by
	AssistedByAssistant = "assistant"
	AssistedByAgent     = "agent"

	// A report's window when none is asked for, and the longest it reads
	DefaultReportDays = 30
	MaxReportDays     = 365

	EventAssistantTurn = "assistant_turn"
	EventAssistedOrder = "assisted_order"
	EventOrderPlaced   = "order_placed"
)
