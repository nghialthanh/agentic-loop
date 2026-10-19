package kafka

// Locked topic and consumer group names (spec / foundation).
const (
	TopicInput  = "chat.customer.messages"
	TopicOutput = "chat.agent.replies"
	TopicDLQ    = "chat.customer.messages.dlq"
	GroupID     = "agentic-loop"
)
