package observability_test

import (
	"strings"
	"testing"

	"github.com/agentic-loop/worker/internal/adapter/observability"
)

func TestRedactPII_replacesEmailPhoneAndCard(t *testing.T) {
	in := "Contact ada@example.com or +1-555-0100; card 4111111111111111"
	out := observability.RedactPII(in)
	if strings.Contains(out, "ada@example.com") {
		t.Errorf("email not redacted: %q", out)
	}
	if strings.Contains(out, "555-0100") || strings.Contains(out, "5550100") {
		t.Errorf("phone not redacted: %q", out)
	}
	if strings.Contains(out, "4111111111111111") {
		t.Errorf("card not redacted: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected [REDACTED] markers, got %q", out)
	}
}
