package attempt_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/service/attempt"
	"github.com/agentic-loop/worker/internal/repository/catalog"
	"github.com/agentic-loop/worker/internal/contract"
)

type errSelectModel struct {
	err error
}

func (m errSelectModel) SelectSkills(context.Context, attempt.SkillSelectionCall) (string, error) {
	return "", m.err
}

func (m errSelectModel) CallWithTools(context.Context, attempt.ToolCallingCall) (attempt.ToolCallingResult, error) {
	return attempt.ToolCallingResult{}, errors.New("should not call tools")
}

type errToolModel struct {
	selectionJSON string
	toolErr       error
}

func (m errToolModel) SelectSkills(_ context.Context, call attempt.SkillSelectionCall) (string, error) {
	return m.selectionJSON, nil
}

func (m errToolModel) CallWithTools(context.Context, attempt.ToolCallingCall) (attempt.ToolCallingResult, error) {
	return attempt.ToolCallingResult{}, m.toolErr
}

func TestRun_selectSkillsErrorStoresNoReply(t *testing.T) {
	cat := &toolAwareCatalog{skills: []catalog.Skill{orderSupportCandidate()}}
	model := errSelectModel{err: errors.New("model unavailable")}
	api := &recordingOrderAPI{}
	store := newRecordingStore()
	a := attempt.New(attempt.Deps{
		Catalog:   cat,
		Model:     model,
		Tools:     api,
		Store:     store,
		Clock:     fakeClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)},
		ModelName: "gpt-test",
	})

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v (want stored output, not bare error)", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected stored output")
	}
	if out.Error == nil || out.Error.Code != "no_reply" {
		t.Errorf("error = %+v, want no_reply", out.Error)
	}
	if out.Reply != attempt.TimeoutReply {
		t.Errorf("reply = %q, want %q", out.Reply, attempt.TimeoutReply)
	}
	if out.Model != "gpt-test" {
		t.Errorf("model = %q, want gpt-test (selection was attempted)", out.Model)
	}
	// Redelivery must see the stored no_reply without calling the model again.
	result2, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("redelivery Run: %v", err)
	}
	if result2.Output == nil || result2.Output.Error == nil || result2.Output.Error.Code != "no_reply" {
		t.Errorf("stored redelivery = %+v", result2.Output)
	}
}

func TestRun_callWithToolsErrorStoresNoReply(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools()[:1],
	}
	model := errToolModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolErr:       errors.New("provider 500"),
	}
	api := &recordingOrderAPI{}
	store := newRecordingStore()
	a := attempt.New(attempt.Deps{
		Catalog:   cat,
		Model:     model,
		Tools:     api,
		Store:     store,
		Clock:     fakeClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)},
		ModelName: "gpt-test",
	})

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if out.Error == nil || out.Error.Code != "no_reply" {
		t.Errorf("error = %+v, want no_reply", out.Error)
	}
	if out.Model != "gpt-test" {
		t.Errorf("model = %q", out.Model)
	}
}

func TestRun_setsModelWhenModelCalled(t *testing.T) {
	cat := &toolAwareCatalog{skills: []catalog.Skill{orderSupportCandidate()}}
	model := &scriptedModel{selectionJSON: `{"intent":"greeting","skill_names":[],"reason":"none"}`}
	store := newRecordingStore()
	a := attempt.New(attempt.Deps{
		Catalog:   cat,
		Model:     model,
		Tools:     &recordingOrderAPI{},
		Store:     store,
		Clock:     fakeClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)},
		ModelName: "gpt-4o-mini",
	})

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Output.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want gpt-4o-mini", result.Output.Model)
	}
}

func TestRun_invalidToolArgsRefuseHTTPWithoutCountingExecution(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools()[:1],
	}
	// Four invalid-arg attempts then a final reply — must not hit the tool cap via HTTP.
	var responses []attempt.ToolCallingResult
	for i := 0; i < attempt.MaxToolExecutions+2; i++ {
		responses = append(responses, attempt.ToolCallingResult{
			ToolCalls: []attempt.ToolCall{{
				ID:        "call",
				Name:      "get_order_status",
				Arguments: json.RawMessage(`{}`), // missing required order_id
			}},
		})
	}
	responses = append(responses, attempt.ToolCallingResult{Reply: "I need an order id."})
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: responses,
	}
	api := &recordingOrderAPI{}
	a, _ := newReactAttempt(t, cat, model, api)

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(api.requests) != 0 {
		t.Fatalf("HTTP must not run on invalid args, got %d requests", len(api.requests))
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if len(out.UsedTools) == 0 {
		t.Fatal("expected used_tools error entries")
	}
	for _, u := range out.UsedTools {
		if u.Status != "error" {
			t.Errorf("used_tool = %#v, want error", u)
		}
	}
}

func TestRun_unmetRequiresDoesNotCountTowardToolCap(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	var responses []attempt.ToolCallingResult
	// More unmet-requires refusals than MaxToolExecutions, then a successful status read.
	for i := 0; i < attempt.MaxToolExecutions+2; i++ {
		responses = append(responses, attempt.ToolCallingResult{
			ToolCalls: []attempt.ToolCall{{
				ID:        "refund",
				Name:      "refund_order",
				Arguments: json.RawMessage(`{"order_id":"1001","reason":"requested"}`),
			}},
		})
	}
	responses = append(responses,
		attempt.ToolCallingResult{
			ToolCalls: []attempt.ToolCall{{
				ID:        "status",
				Name:      "get_order_status",
				Arguments: json.RawMessage(`{"order_id":"1001"}`),
			}},
		},
		attempt.ToolCallingResult{Reply: "Order 1001 is shipped via UPS."},
	)
	model := &scriptedModel{
		selectionJSON: `{"intent":"refund","skill_names":["order_support"],"reason":"refund"}`,
		toolResponses: responses,
	}
	api := &recordingOrderAPI{
		body:    json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
		latency: 10,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	in := validInput()
	in.Content = "Please refund order 1001"
	result, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(api.requests) != 1 {
		t.Fatalf("HTTP requests = %d, want 1 status read after unmet requires (requires must not burn cap)", len(api.requests))
	}
	if api.requests[0].Name != "get_order_status" {
		t.Errorf("request = %#v", api.requests[0])
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if !strings.Contains(out.Reply, "1001") {
		t.Errorf("reply = %q", out.Reply)
	}
}

func TestRun_passesAuthAndTimeoutToToolExecutor(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools()[:1],
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
		latency: 20,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	if _, err := a.Run(context.Background(), validInput()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(api.requests) != 1 {
		t.Fatalf("requests = %d", len(api.requests))
	}
	req := api.requests[0]
	if req.Auth != "bearer" {
		t.Errorf("auth = %q, want bearer", req.Auth)
	}
	if req.TimeoutMS != 3000 {
		t.Errorf("timeout_ms = %d, want 3000", req.TimeoutMS)
	}
}

func TestRun_customerMessageDigitsNotAllowedAccountFacts(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools()[:1],
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
			// Model echoes the customer order id and invents status — keep has different id.
			{Reply: "Order 1001 is delivered."},
		},
	}
	api := &recordingOrderAPI{
		body:    json.RawMessage(`{"order_id":"9999","status":"shipped","carrier":"UPS"}`),
		latency: 10,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	in := validInput()
	in.Content = "Where is order 1001?"
	result, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	// Customer digit "1001" must not authorize inventing facts about it without a matching keep.
	if strings.Contains(out.Reply, "1001") && strings.Contains(strings.ToLower(out.Reply), "delivered") {
		t.Errorf("reply used customer message digits as allowed facts: %q", out.Reply)
	}
	if result.Hallucination {
		// Hallucination flag expected when composer had to replace invented content.
	}
}

func TestRun_selectionPromptUsesCustomerField(t *testing.T) {
	cat := &fakeCatalog{skills: []catalog.Skill{orderSupportCandidate()}}
	model := &fakeModel{selectionJSON: `{"intent":"general","skill_names":[],"reason":"none"}`}
	a := newAttemptWith(t, cat, model, newRecordingStore())

	in := validInput()
	if _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(model.calls) != 1 {
		t.Fatalf("calls = %d", len(model.calls))
	}
	if !strings.Contains(model.calls[0].Customer, in.Content) {
		t.Errorf("Customer prompt missing content, got %q", model.calls[0].Customer)
	}
}

func TestValidateOutput_rejectsInvalidShape(t *testing.T) {
	err := contract.ValidateOutput(contract.OutputMessage{
		MessageID: "",
		Reply:     "hi",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
