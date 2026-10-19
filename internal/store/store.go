package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/agentic-loop/worker/internal/contract"
)

// DefaultClaimTTL is how long a live claim blocks other workers (recommended default).
const DefaultClaimTTL = 90 * time.Second

// DefaultRateLimit is max accepted messages per customer per rolling window.
const DefaultRateLimit = 10

// DefaultRateWindow is the rolling window for per-customer rate limits.
const DefaultRateWindow = time.Minute

// RateLimitedReply is the fixed customer-facing text when over the rate limit.
const RateLimitedReply = "I'm getting too many messages right now. Please try again in a minute."

// ErrClaimHeld means another worker holds a non-expired claim for the message.
var ErrClaimHeld = errors.New("claim held by another worker")

// ClaimOutcome is the result of trying to claim a message_id.
type ClaimOutcome int

const (
	// ClaimWon means this worker owns processing (new claim or takeover after expiry).
	ClaimWon ClaimOutcome = iota
	// ClaimStored means a reply is already stored; republish Stored without tools.
	ClaimStored
	// ClaimHeld means another worker holds a live claim.
	ClaimHeld
)

// ClaimResult is returned from Claim.
type ClaimResult struct {
	Outcome ClaimOutcome
	Stored  *contract.OutputMessage
}

// Memory is an in-process shared store for tests (claims, outputs, rate limits).
type Memory struct {
	mu       sync.Mutex
	claims   map[string]memoryClaim
	rateHits map[string][]rateHit // customer_id -> hits
	ClaimTTL time.Duration
	Limit    int
	Window   time.Duration
}

type memoryClaim struct {
	expiresAt time.Time
	output    *contract.OutputMessage
}

type rateHit struct {
	messageID string
	at        time.Time
}

// NewMemory builds an empty in-memory store with defaults.
func NewMemory() *Memory {
	return &Memory{
		claims:   make(map[string]memoryClaim),
		rateHits: make(map[string][]rateHit),
		ClaimTTL: DefaultClaimTTL,
		Limit:    DefaultRateLimit,
		Window:   DefaultRateWindow,
	}
}

// Claim acquires or observes the claim for messageID at now.
func (m *Memory) Claim(_ context.Context, messageID string, now time.Time) (ClaimResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.claims[messageID]
	if !ok {
		m.claims[messageID] = memoryClaim{expiresAt: now.Add(m.claimTTL())}
		return ClaimResult{Outcome: ClaimWon}, nil
	}
	if c.output != nil {
		out := *c.output
		return ClaimResult{Outcome: ClaimStored, Stored: &out}, nil
	}
	if now.Before(c.expiresAt) {
		return ClaimResult{Outcome: ClaimHeld}, nil
	}
	// Expired claim: take over.
	m.claims[messageID] = memoryClaim{expiresAt: now.Add(m.claimTTL())}
	return ClaimResult{Outcome: ClaimWon}, nil
}

// SaveOutput stores the output for a claimed message_id.
func (m *Memory) SaveOutput(_ context.Context, out contract.OutputMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.claims[out.MessageID]
	cp := out
	c.output = &cp
	m.claims[out.MessageID] = c
	return nil
}

// ReserveRateLimit counts messageID for customerID if under the rolling limit.
// Returns false when the customer is already at/over the limit (does not record).
// Duplicate message_id does not count again.
func (m *Memory) ReserveRateLimit(_ context.Context, customerID, messageID string, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hits := m.rateHits[customerID]
	windowStart := now.Add(-m.window())
	trimmed := hits[:0]
	for _, h := range hits {
		if !h.at.Before(windowStart) {
			trimmed = append(trimmed, h)
		}
	}
	for _, h := range trimmed {
		if h.messageID == messageID {
			m.rateHits[customerID] = trimmed
			return true, nil
		}
	}
	if len(trimmed) >= m.limit() {
		m.rateHits[customerID] = trimmed
		return false, nil
	}
	m.rateHits[customerID] = append(trimmed, rateHit{messageID: messageID, at: now})
	return true, nil
}

func (m *Memory) claimTTL() time.Duration {
	if m.ClaimTTL <= 0 {
		return DefaultClaimTTL
	}
	return m.ClaimTTL
}

func (m *Memory) limit() int {
	if m.Limit <= 0 {
		return DefaultRateLimit
	}
	return m.Limit
}

func (m *Memory) window() time.Duration {
	if m.Window <= 0 {
		return DefaultRateWindow
	}
	return m.Window
}
