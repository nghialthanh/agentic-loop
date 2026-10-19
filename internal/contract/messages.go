package contract

import "time"

// HistoryTurn is one prior turn in conversation_history.
type HistoryTurn struct {
	Role    string     `json:"role"`
	Content string     `json:"content"`
	At      *time.Time `json:"at,omitempty"`
}

// InputMessage is the Kafka payload on chat.customer.messages.
type InputMessage struct {
	MessageID           string        `json:"message_id"`
	ConversationID      string        `json:"conversation_id"`
	CustomerID          string        `json:"customer_id"`
	Content             string        `json:"content"`
	ConversationHistory []HistoryTurn `json:"conversation_history"`
}

// UsedTool records one tool invocation on the output (no raw API bodies).
type UsedTool struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	LatencyMs int    `json:"latency_ms"`
}

// OutputError is set when the attempt failed after accepting the input.
type OutputError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OutputMessage is the Kafka payload on chat.agent.replies.
type OutputMessage struct {
	MessageID      string       `json:"message_id"`
	ConversationID string       `json:"conversation_id"`
	Reply          string       `json:"reply"`
	Intent         string       `json:"intent"`
	Skills         []string     `json:"skills"`
	UsedTools      []UsedTool   `json:"used_tools"`
	Confidence     float64      `json:"confidence"`
	Error          *OutputError `json:"error"`
	TraceID        string       `json:"trace_id"`
	Model          string       `json:"model"`
}

// DeadLetter is published to chat.customer.messages.dlq for malformed input.
type DeadLetter struct {
	Payload any    `json:"payload"`
	Reason  string `json:"reason"`
}
