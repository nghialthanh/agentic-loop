package observability

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// AttemptFields are the required correlation ids on every attempt log line.
type AttemptFields struct {
	MessageID      string
	ConversationID string
	TraceID        string
}

// Logger writes structured JSON logs with attempt correlation ids and PII redaction.
type Logger struct {
	mu  sync.Mutex
	out io.Writer
}

// NewLogger writes JSON lines to out.
func NewLogger(out io.Writer) *Logger {
	return &Logger{out: out}
}

// AttemptInfo logs msg with message_id, conversation_id, and trace_id.
// Extra key/value pairs are redacted when string-valued.
func (l *Logger) AttemptInfo(msg string, fields AttemptFields, kv ...any) {
	if l == nil || l.out == nil {
		return
	}
	rec := map[string]any{
		"msg":             msg,
		"level":           "info",
		"time":            time.Now().UTC().Format(time.RFC3339Nano),
		"message_id":      fields.MessageID,
		"conversation_id": fields.ConversationID,
		"trace_id":        fields.TraceID,
	}
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			continue
		}
		rec[key] = redactValue(kv[i+1])
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(b, '\n'))
}

func redactValue(v any) any {
	switch t := v.(type) {
	case string:
		return RedactPII(t)
	default:
		return v
	}
}
