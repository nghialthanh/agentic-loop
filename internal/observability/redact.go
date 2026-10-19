package observability

import "regexp"

var (
	emailRE = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	// Phone: optional +, digits/spaces/dashes totaling 10+ digits.
	phoneRE = regexp.MustCompile(`(?i)(?:\+?\d[\d\-.\s]{8,}\d)`)
	cardRE  = regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`)
)

const redacted = "[REDACTED]"

// RedactPII replaces emails, phone numbers, and card-like digit runs.
func RedactPII(s string) string {
	out := emailRE.ReplaceAllString(s, redacted)
	out = phoneRE.ReplaceAllString(out, redacted)
	out = cardRE.ReplaceAllString(out, redacted)
	return out
}
