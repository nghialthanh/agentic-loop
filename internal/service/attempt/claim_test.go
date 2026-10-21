package attempt_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/service/attempt"
	"github.com/agentic-loop/worker/internal/repository/catalog"
	"github.com/agentic-loop/worker/internal/repository/store"
)

type mutableClock struct {
	now time.Time
}

func (c *mutableClock) Now() time.Time { return c.now }

func TestRun_duplicateDeliveryRepublishesStoredReplyWithoutTools(t *testing.T) {
	mem := store.NewMemory()
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{Reply: "Order 1001 is shipped via UPS."},
		},
	}
	api := &recordingOrderAPI{
		body:    json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
		latency: 50,
	}
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	a := attempt.New(attempt.Deps{
		Catalog: cat,
		Model:   model,
		Tools:   api,
		Store:   mem,
		Clock:   fakeClock{now: fixed},
	})

	first, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.Output == nil {
		t.Fatal("expected first output")
	}
	if len(api.requests) != 1 {
		t.Fatalf("first pass tool HTTP calls = %d, want 1", len(api.requests))
	}
	if len(model.selectionCalls) != 1 {
		t.Fatalf("first pass selection calls = %d, want 1", len(model.selectionCalls))
	}

	// Redelivery: same message_id, shared store already has the reply.
	second, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second.Output == nil {
		t.Fatal("expected stored output on redelivery")
	}
	if second.Output.Reply != first.Output.Reply {
		t.Errorf("redelivery reply = %q, want stored %q", second.Output.Reply, first.Output.Reply)
	}
	if second.Output.TraceID != first.Output.TraceID {
		t.Errorf("redelivery must republish the stored record, trace_id %q vs %q", second.Output.TraceID, first.Output.TraceID)
	}
	if len(api.requests) != 1 {
		t.Fatalf("redelivery must not call tools again, HTTP calls = %d", len(api.requests))
	}
	if len(model.selectionCalls) != 1 {
		t.Fatalf("redelivery must not call model again, selection calls = %d", len(model.selectionCalls))
	}
	if len(model.toolCalls) != len(model.toolResponses) {
		// toolCalls should stay at first-pass count (2: tool request + final reply)
		t.Fatalf("redelivery must not add tool-calling model calls, got %d", len(model.toolCalls))
	}
}

func TestRun_claimHeldBlocksUntilExpiryThenTakeover(t *testing.T) {
	mem := store.NewMemory()
	mem.ClaimTTL = 30 * time.Second
	clock := &mutableClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}

	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	api := &recordingOrderAPI{
		body: json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
	}

	winnerModel := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{Reply: "Order 1001 is shipped via UPS."},
		},
	}

	// Worker A wins the claim but "crashes" before saving output: claim only.
	if _, err := mem.Claim(context.Background(), validInput().MessageID, clock.Now()); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	other := attempt.New(attempt.Deps{
		Catalog: cat,
		Model:   winnerModel,
		Tools:   api,
		Store:   mem,
		Clock:   clock,
	})
	_, err := other.Run(context.Background(), validInput())
	if !errors.Is(err, store.ErrClaimHeld) {
		t.Fatalf("while claim live: err = %v, want ErrClaimHeld", err)
	}
	if len(api.requests) != 0 {
		t.Fatalf("held claim must not call tools, got %d", len(api.requests))
	}

	// After expiry, another worker may take over and finish.
	clock.now = clock.now.Add(mem.ClaimTTL + time.Second)
	result, err := other.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("takeover Run: %v", err)
	}
	if result.Output == nil {
		t.Fatal("expected takeover output")
	}
	if len(api.requests) != 1 {
		t.Fatalf("takeover tool HTTP calls = %d, want 1", len(api.requests))
	}
	if result.Output.Reply != "Order 1001 is shipped via UPS." {
		t.Errorf("takeover reply = %q", result.Output.Reply)
	}
}

func TestRun_overLimitCustomerGetsRateLimitedReplyWithoutModelOrTools(t *testing.T) {
	mem := store.NewMemory()
	mem.Limit = 2
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{Reply: "should not run"},
		},
	}
	api := &recordingOrderAPI{
		body: json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
	}
	a := attempt.New(attempt.Deps{
		Catalog: cat,
		Model:   model,
		Tools:   api,
		Store:   mem,
		Clock:   fakeClock{now: fixed},
	})

	// Fill the rolling window with two distinct message ids for the same customer.
	for i, msgID := range []string{"msg_rl_1", "msg_rl_2"} {
		in := validInput()
		in.MessageID = msgID
		in.Content = "ping"
		// Force general path so we do not need tool scripting for fillers.
		fillerModel := &fakeModel{selectionJSON: `{"intent":"general","skill_names":[],"reason":"none"}`}
		filler := attempt.New(attempt.Deps{
			Catalog: cat,
			Model:   fillerModel,
			Tools:   api,
			Store:   mem,
			Clock:   fakeClock{now: fixed.Add(time.Duration(i) * time.Second)},
		})
		if _, err := filler.Run(context.Background(), in); err != nil {
			t.Fatalf("filler %s: %v", msgID, err)
		}
	}

	over := validInput()
	over.MessageID = "msg_rl_over"
	result, err := a.Run(context.Background(), over)
	if err != nil {
		t.Fatalf("over-limit Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected rate-limited output")
	}
	if out.Reply != attempt.RateLimitedReply {
		t.Errorf("reply = %q, want rate-limit sentence", out.Reply)
	}
	if out.Error == nil || out.Error.Code != "rate_limited" {
		t.Errorf("error = %+v, want rate_limited", out.Error)
	}
	if len(out.Skills) != 0 || len(out.UsedTools) != 0 {
		t.Errorf("skills/used_tools must be empty, got skills=%v tools=%v", out.Skills, out.UsedTools)
	}
	if len(model.selectionCalls) != 0 {
		t.Fatalf("rate limit must not call model, selection=%d", len(model.selectionCalls))
	}
	if len(api.requests) != 0 {
		t.Fatalf("rate limit must not call tools, HTTP=%d", len(api.requests))
	}

	// Redelivery of the rate-limited message republishes without counting again
	// and without model/tools — even after the window would allow more.
	redelivery, err := a.Run(context.Background(), over)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if redelivery.Output == nil || redelivery.Output.Error == nil || redelivery.Output.Error.Code != "rate_limited" {
		t.Fatalf("redelivery output = %+v", redelivery.Output)
	}
	if len(model.selectionCalls) != 0 || len(api.requests) != 0 {
		t.Fatal("redelivery of rate-limited reply must not touch model or tools")
	}

	// A new message_id still counts only once; after redelivery the limit remains full.
	stillOver := validInput()
	stillOver.MessageID = "msg_rl_still"
	again, err := a.Run(context.Background(), stillOver)
	if err != nil {
		t.Fatalf("still over: %v", err)
	}
	if again.Output == nil || again.Output.Error == nil || again.Output.Error.Code != "rate_limited" {
		t.Fatalf("expected still rate_limited, got %+v", again.Output)
	}
}

func TestRun_claimIsTakenBeforeToolHTTP(t *testing.T) {
	mem := store.NewMemory()
	var claimBeforeTool bool
	api := &recordingOrderAPI{
		body: json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
	}
	tools := &claimOrderAPI{
		inner: api,
		onExecute: func() {
			// At first tool call, claim row must already exist (won).
			res, err := mem.Claim(context.Background(), validInput().MessageID, time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))
			if err != nil {
				t.Errorf("claim during tool: %v", err)
				return
			}
			// Live claim without stored output → Held for a second observer.
			claimBeforeTool = res.Outcome == store.ClaimHeld
		},
	}
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{Reply: "Order 1001 is shipped via UPS."},
		},
	}
	a := attempt.New(attempt.Deps{
		Catalog: cat,
		Model:   model,
		Tools:   tools,
		Store:   mem,
		Clock:   fakeClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)},
	})
	if _, err := a.Run(context.Background(), validInput()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !claimBeforeTool {
		t.Fatal("claim must be live before the first tool HTTP call")
	}
	if len(api.requests) != 1 {
		t.Fatalf("tool HTTP calls = %d", len(api.requests))
	}
}

type claimOrderAPI struct {
	inner     *recordingOrderAPI
	onExecute func()
}

func (a *claimOrderAPI) Execute(ctx context.Context, req attempt.ToolHTTPRequest) attempt.ToolHTTPResponse {
	if a.onExecute != nil {
		a.onExecute()
	}
	return a.inner.Execute(ctx, req)
}
