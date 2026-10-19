package contract

import (
	"fmt"
	"strings"
)

// knownErrorCodes are the foundation brief output error codes.
var knownErrorCodes = map[string]struct{}{
	"rate_limited":  {},
	"guard_blocked": {},
	"tool_failed":   {},
	"timeout":       {},
	"no_reply":      {},
}

// ValidateOutput checks the output message shape before Kafka publish.
// Invalid shapes must not be published.
func ValidateOutput(out OutputMessage) error {
	if strings.TrimSpace(out.MessageID) == "" {
		return fmt.Errorf("message_id is required")
	}
	if strings.TrimSpace(out.ConversationID) == "" {
		return fmt.Errorf("conversation_id is required")
	}
	if strings.TrimSpace(out.Reply) == "" {
		return fmt.Errorf("reply is required")
	}
	if out.Skills == nil {
		return fmt.Errorf("skills must be a non-nil slice")
	}
	if out.UsedTools == nil {
		return fmt.Errorf("used_tools must be a non-nil slice")
	}
	if out.Confidence < 0 || out.Confidence > 1 {
		return fmt.Errorf("confidence out of range: %v", out.Confidence)
	}
	if strings.TrimSpace(out.TraceID) == "" {
		return fmt.Errorf("trace_id is required")
	}
	// model may be empty when the model was never called (rate limit / guard)
	if out.Error != nil {
		if strings.TrimSpace(out.Error.Code) == "" {
			return fmt.Errorf("error.code is required when error is set")
		}
		if _, ok := knownErrorCodes[out.Error.Code]; !ok {
			return fmt.Errorf("unknown error.code %q", out.Error.Code)
		}
		if strings.TrimSpace(out.Error.Message) == "" {
			return fmt.Errorf("error.message is required when error is set")
		}
	}
	for i, u := range out.UsedTools {
		if strings.TrimSpace(u.Name) == "" {
			return fmt.Errorf("used_tools[%d].name is required", i)
		}
		if u.Status != "ok" && u.Status != "error" {
			return fmt.Errorf("used_tools[%d].status must be ok or error", i)
		}
	}
	return nil
}
