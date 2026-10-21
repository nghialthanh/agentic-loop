package attempt_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/service/attempt"
	"github.com/agentic-loop/worker/internal/repository/catalog"
	"github.com/agentic-loop/worker/internal/contract"
)

type toolAwareCatalog struct {
	skills        []catalog.Skill
	tools         []catalog.Tool
	searchCalls   int
	toolsForCalls [][]string
}

func (c *toolAwareCatalog) SearchSkills(_ context.Context, _ string) ([]catalog.Skill, error) {
	c.searchCalls++
	out := make([]catalog.Skill, len(c.skills))
	copy(out, c.skills)
	return out, nil
}

func (c *toolAwareCatalog) ToolsForSkills(_ context.Context, skillNames []string) ([]catalog.Tool, error) {
	c.toolsForCalls = append(c.toolsForCalls, append([]string(nil), skillNames...))
	var out []catalog.Tool
	for _, tool := range c.tools {
		if !tool.Enabled {
			continue
		}
		out = append(out, tool)
	}
	return out, nil
}

type scriptedModel struct {
	selectionJSON string
	selectionCalls []attempt.SkillSelectionCall
	toolResponses  []attempt.ToolCallingResult
	toolCalls      []attempt.ToolCallingCall
	toolCallIdx    int
}

func (m *scriptedModel) SelectSkills(_ context.Context, call attempt.SkillSelectionCall) (string, error) {
	m.selectionCalls = append(m.selectionCalls, call)
	return m.selectionJSON, nil
}

func (m *scriptedModel) CallWithTools(_ context.Context, call attempt.ToolCallingCall) (attempt.ToolCallingResult, error) {
	m.toolCalls = append(m.toolCalls, call)
	if m.toolCallIdx >= len(m.toolResponses) {
		return attempt.ToolCallingResult{Reply: "I am not sure."}, nil
	}
	res := m.toolResponses[m.toolCallIdx]
	m.toolCallIdx++
	return res, nil
}

type recordingOrderAPI struct {
	requests []attempt.ToolHTTPRequest
	body     json.RawMessage
	latency  int
	err      error
	bodies   []json.RawMessage // optional per-call bodies
}

func (a *recordingOrderAPI) Execute(_ context.Context, req attempt.ToolHTTPRequest) attempt.ToolHTTPResponse {
	a.requests = append(a.requests, req)
	if a.err != nil {
		return attempt.ToolHTTPResponse{LatencyMs: a.latency, Err: a.err}
	}
	body := a.body
	if len(a.bodies) > 0 {
		idx := len(a.requests) - 1
		if idx < len(a.bodies) {
			body = a.bodies[idx]
		}
	}
	return attempt.ToolHTTPResponse{Body: body, LatencyMs: a.latency}
}

func orderSupportTools() []catalog.Tool {
	return []catalog.Tool{
		{
			Name:        "get_order_status",
			Description: "Get the status of one order.",
			Method:      "GET",
			Endpoint:    "https://api.internal/orders/{order_id}",
			Auth:        "bearer",
			TimeoutMS:   3000,
			Enabled:     true,
			Parameters:  json.RawMessage(`{"type":"object","additionalProperties":false,"required":["order_id"],"properties":{"order_id":{"type":"string"}}}`),
			Response:    json.RawMessage(`{"keep":["order_id","status","carrier"]}`),
			Requires:    []string{},
		},
		{
			Name:        "refund_order",
			Description: "Refund one order after its status is known.",
			Method:      "POST",
			Endpoint:    "https://api.internal/orders/{order_id}/refunds",
			Auth:        "bearer",
			TimeoutMS:   3000,
			Enabled:     true,
			Parameters:  json.RawMessage(`{"type":"object","additionalProperties":false,"required":["order_id","reason","customer_id"],"properties":{"order_id":{"type":"string"},"reason":{"type":"string"},"customer_id":{"type":"string"}}}`),
			Response:    json.RawMessage(`{"keep":["refund_id","status"]}`),
			Requires:    []string{"get_order_status"},
		},
	}
}

func newReactAttempt(t *testing.T, cat attempt.Catalog, model attempt.Model, tools attempt.Tools) (*attempt.Attempt, *recordingStore) {
	t.Helper()
	rec := newRecordingStore()
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	return attempt.New(attempt.Deps{
		Catalog: cat,
		Model:   model,
		Tools:   tools,
		Store:   rec,
		Clock:   fakeClock{now: fixed},
	}), rec
}

func TestRun_statusQuestionGroundsReplyFromOrderTool(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"asks where order is"}`,
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
		body:    json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS","internal_note":"ignore-me"}`),
		latency: 120,
	}
	a, store := newReactAttempt(t, cat, model, api)

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.DeadLetter != nil {
		t.Fatalf("unexpected dead letter: %+v", result.DeadLetter)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}

	if out.Reply != "Order 1001 is shipped via UPS." {
		t.Errorf("reply = %q", out.Reply)
	}
	if !strings.Contains(out.Reply, "1001") || !strings.Contains(out.Reply, "shipped") {
		t.Errorf("reply missing grounded facts: %q", out.Reply)
	}
	if strings.Contains(out.Reply, "ignore-me") {
		t.Error("reply must not include non-kept tool fields")
	}
	if len(out.Skills) != 1 || out.Skills[0] != "order_support" {
		t.Errorf("skills = %v", out.Skills)
	}
	if out.Intent != "order_status" {
		t.Errorf("intent = %q", out.Intent)
	}
	if len(out.UsedTools) != 1 {
		t.Fatalf("used_tools = %#v, want 1", out.UsedTools)
	}
	ut := out.UsedTools[0]
	if ut.Name != "get_order_status" || ut.Status != "ok" || ut.LatencyMs != 120 {
		t.Errorf("used_tools[0] = %#v", ut)
	}
	if out.Error != nil {
		t.Errorf("error want nil, got %+v", out.Error)
	}
	if out.Confidence <= 0.2 {
		t.Errorf("confidence = %v, want grounded (>0.2)", out.Confidence)
	}
	if len(store.claims) != 1 {
		t.Errorf("claims = %v", store.claims)
	}

	if len(model.selectionCalls) != 1 {
		t.Fatalf("selection calls = %d", len(model.selectionCalls))
	}
	if len(model.toolCalls) < 1 {
		t.Fatal("expected tool-calling model calls")
	}
	first := model.toolCalls[0]
	if first.System != attempt.ToolCallingSystemPrompt {
		t.Error("tool-calling prompt must use ToolCallingSystemPrompt")
	}
	if !strings.Contains(strings.ToLower(first.System), "explicit") {
		t.Error("tool-calling prompt must include the explicit-request rule")
	}
	if len(first.Tools) != 2 {
		t.Fatalf("function schemas = %d, want 2 enabled tools", len(first.Tools))
	}
	names := map[string]bool{}
	for _, schema := range first.Tools {
		names[schema.Name] = true
		if schema.Description == "" || len(schema.Parameters) == 0 {
			t.Errorf("schema %#v incomplete", schema)
		}
	}
	if !names["get_order_status"] || !names["refund_order"] {
		t.Errorf("schemas = %v", names)
	}

	if len(api.requests) != 1 {
		t.Fatalf("order API requests = %d, want 1", len(api.requests))
	}
	req := api.requests[0]
	if req.Name != "get_order_status" || req.Method != "GET" {
		t.Errorf("request = %#v", req)
	}
	if req.Endpoint != "https://api.internal/orders/1001" {
		t.Errorf("endpoint = %q", req.Endpoint)
	}
	if len(cat.toolsForCalls) != 1 || len(cat.toolsForCalls[0]) != 1 || cat.toolsForCalls[0][0] != "order_support" {
		t.Errorf("ToolsForSkills calls = %#v", cat.toolsForCalls)
	}
}

func TestRun_replyDoesNotInventAccountFactsAbsentFromTools(t *testing.T) {
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
			{Reply: "Order 9999 is cancelled and your balance is $50."},
		},
	}
	api := &recordingOrderAPI{
		body:    json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
		latency: 80,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if strings.Contains(out.Reply, "9999") || strings.Contains(strings.ToLower(out.Reply), "cancelled") {
		t.Errorf("reply invents facts: %q", out.Reply)
	}
	if strings.Contains(strings.ToLower(out.Reply), "balance") {
		t.Errorf("reply invents balance: %q", out.Reply)
	}
	if !strings.Contains(out.Reply, "1001") || !strings.Contains(out.Reply, "shipped") {
		t.Errorf("reply should use tool keep fields, got %q", out.Reply)
	}
}

func TestRun_disabledToolsOmittedFromFunctionSchemas(t *testing.T) {
	tools := orderSupportTools()
	tools[1].Enabled = false
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  tools,
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{Reply: "I can check order status."},
		},
	}
	api := &recordingOrderAPI{}
	a, _ := newReactAttempt(t, cat, model, api)

	if _, err := a.Run(context.Background(), validInput()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(model.toolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(model.toolCalls))
	}
	if len(model.toolCalls[0].Tools) != 1 || model.toolCalls[0].Tools[0].Name != "get_order_status" {
		t.Errorf("schemas = %#v, want only get_order_status", model.toolCalls[0].Tools)
	}
}

func TestRun_historyBudgetKeepsNewestTurnsOnlyForThisCall(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools()[:1],
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{Reply: "Thanks, noted."},
		},
	}
	api := &recordingOrderAPI{}
	a, _ := newReactAttempt(t, cat, model, api)

	in := validInput()
	in.ConversationHistory = nil
	for i := 0; i < 20; i++ {
		role := "customer"
		if i%2 == 1 {
			role = "agent"
		}
		in.ConversationHistory = append(in.ConversationHistory, contract.HistoryTurn{
			Role:    role,
			Content: fmt.Sprintf("old-turn-%02d", i),
		})
	}

	if _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(model.toolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(model.toolCalls))
	}
	joined := ""
	for _, msg := range model.toolCalls[0].Messages {
		joined += msg.Content + "\n"
	}
	if strings.Contains(joined, "old-turn-00") || strings.Contains(joined, "old-turn-07") {
		t.Errorf("older turns should be omitted, transcript=%q", joined)
	}
	if !strings.Contains(joined, "old-turn-19") {
		t.Errorf("newest turn missing, transcript=%q", joined)
	}
	if !strings.Contains(joined, attempt.HistoryOmissionLine) {
		t.Errorf("expected omission line, transcript=%q", joined)
	}
	// Original input history must remain unchanged for the caller.
	if len(in.ConversationHistory) != 20 {
		t.Errorf("input history mutated: len=%d", len(in.ConversationHistory))
	}
	if in.ConversationHistory[0].Content != "old-turn-00" {
		t.Errorf("input history[0] = %q", in.ConversationHistory[0].Content)
	}
}

func TestRun_atMostFourToolExecutionsPerAttempt(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools()[:1],
	}
	var responses []attempt.ToolCallingResult
	for i := 0; i < 6; i++ {
		responses = append(responses, attempt.ToolCallingResult{
			ToolCalls: []attempt.ToolCall{{
				ID:        fmt.Sprintf("call_%d", i),
				Name:      "get_order_status",
				Arguments: json.RawMessage(fmt.Sprintf(`{"order_id":"%d"}`, 1000+i)),
			}},
		})
	}
	responses = append(responses, attempt.ToolCallingResult{Reply: "Stopping after budget."})
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: responses,
	}
	api := &recordingOrderAPI{
		body:    json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
		latency: 10,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if len(api.requests) > attempt.MaxToolExecutions {
		t.Fatalf("HTTP executions = %d, want <= %d", len(api.requests), attempt.MaxToolExecutions)
	}
	if len(out.UsedTools) > attempt.MaxToolExecutions {
		t.Fatalf("used_tools = %d, want <= %d", len(out.UsedTools), attempt.MaxToolExecutions)
	}
	if len(api.requests) != attempt.MaxToolExecutions {
		t.Errorf("HTTP executions = %d, want exactly %d", len(api.requests), attempt.MaxToolExecutions)
	}
}

func TestRun_generalAnswerSkipsToolCallingPrompt(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"greeting","skill_names":[],"reason":"none"}`,
	}
	api := &recordingOrderAPI{}
	a, _ := newReactAttempt(t, cat, model, api)

	result, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertGeneralAnswer(t, result.Output)
	if len(model.toolCalls) != 0 {
		t.Fatalf("tool-calling must not run without accepted skills, calls=%d", len(model.toolCalls))
	}
	if len(api.requests) != 0 {
		t.Fatalf("order API must not be called, requests=%d", len(api.requests))
	}
	if len(cat.toolsForCalls) != 0 {
		t.Fatalf("tools must not load without skills, calls=%#v", cat.toolsForCalls)
	}
}

func TestRun_refundAfterSuccessfulStatusReadCompletes(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"refund","skill_names":["order_support"],"reason":"explicit refund"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_2",
					Name:      "refund_order",
					Arguments: json.RawMessage(`{"order_id":"1001","reason":"customer requested"}`),
				}},
			},
			{Reply: "Refund rf_9 for order 1001 is pending."},
		},
	}
	api := &recordingOrderAPI{
		bodies: []json.RawMessage{
			json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
			json.RawMessage(`{"refund_id":"rf_9","status":"pending"}`),
		},
		latency: 50,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	in := validInput()
	in.Content = "Please refund order 1001"
	result, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if result.DeadLetter != nil {
		t.Fatalf("unexpected dead letter: %+v", result.DeadLetter)
	}
	if len(api.requests) != 2 {
		t.Fatalf("HTTP requests = %d, want 2 (status then refund)", len(api.requests))
	}
	if api.requests[0].Name != "get_order_status" {
		t.Errorf("first HTTP = %q, want get_order_status", api.requests[0].Name)
	}
	if api.requests[1].Name != "refund_order" {
		t.Errorf("second HTTP = %q, want refund_order", api.requests[1].Name)
	}
	if len(out.UsedTools) != 2 {
		t.Fatalf("used_tools = %#v, want 2", out.UsedTools)
	}
	if out.UsedTools[0].Name != "get_order_status" || out.UsedTools[0].Status != "ok" {
		t.Errorf("used_tools[0] = %#v", out.UsedTools[0])
	}
	if out.UsedTools[1].Name != "refund_order" || out.UsedTools[1].Status != "ok" {
		t.Errorf("used_tools[1] = %#v", out.UsedTools[1])
	}
	if !strings.Contains(out.Reply, "rf_9") || !strings.Contains(out.Reply, "1001") {
		t.Errorf("reply = %q, want grounded refund facts", out.Reply)
	}
	if out.Error != nil {
		t.Errorf("error want nil, got %+v", out.Error)
	}
}

func TestRun_refundWithoutRequiredReadRefusesHTTP(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"refund","skill_names":["order_support"],"reason":"refund"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "refund_order",
					Arguments: json.RawMessage(`{"order_id":"1001","reason":"customer requested"}`),
				}},
			},
			{Reply: "I could not complete the refund without checking the order first."},
		},
	}
	api := &recordingOrderAPI{
		body:    json.RawMessage(`{"refund_id":"rf_should_not_run","status":"pending"}`),
		latency: 10,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	in := validInput()
	in.Content = "Please refund order 1001"
	result, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if len(api.requests) != 0 {
		t.Fatalf("HTTP must not run without required read, got %#v", api.requests)
	}
	if len(out.UsedTools) != 1 {
		t.Fatalf("used_tools = %#v, want 1 error", out.UsedTools)
	}
	if out.UsedTools[0].Name != "refund_order" || out.UsedTools[0].Status != "error" {
		t.Errorf("used_tools[0] = %#v, want refund_order error", out.UsedTools[0])
	}
}

func TestRun_customerIDBoundFromMessageNotModel(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"refund","skill_names":["order_support"],"reason":"refund"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{
				ToolCalls: []attempt.ToolCall{{
					ID:   "call_2",
					Name: "refund_order",
					// Model tries to act as another customer; executor must ignore this.
					Arguments: json.RawMessage(`{"order_id":"1001","reason":"requested","customer_id":"cus_OTHER"}`),
				}},
			},
			{Reply: "Refund rf_1 for order 1001 is pending."},
		},
	}
	api := &recordingOrderAPI{
		bodies: []json.RawMessage{
			json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
			json.RawMessage(`{"refund_id":"rf_1","status":"pending"}`),
		},
		latency: 20,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	in := validInput()
	in.CustomerID = "cus_456"
	in.Content = "Please refund order 1001"
	if _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(model.toolCalls) < 1 {
		t.Fatal("expected tool-calling schemas")
	}
	var refundSchema *attempt.FunctionSchema
	for i := range model.toolCalls[0].Tools {
		if model.toolCalls[0].Tools[i].Name == "refund_order" {
			refundSchema = &model.toolCalls[0].Tools[i]
			break
		}
	}
	if refundSchema == nil {
		t.Fatal("refund_order schema missing")
	}
	if strings.Contains(string(refundSchema.Parameters), "customer_id") {
		t.Errorf("model schema must omit customer_id, got %s", refundSchema.Parameters)
	}

	if len(api.requests) != 2 {
		t.Fatalf("HTTP requests = %d, want 2", len(api.requests))
	}
	refundReq := api.requests[1]
	if refundReq.Name != "refund_order" {
		t.Fatalf("second request = %#v", refundReq)
	}
	var args map[string]any
	if err := json.Unmarshal(refundReq.Arguments, &args); err != nil {
		t.Fatalf("unmarshal refund args: %v", err)
	}
	if args["customer_id"] != "cus_456" {
		t.Errorf("customer_id = %v, want message cus_456 (not model-supplied)", args["customer_id"])
	}
}

func TestRun_toolCallingPromptIncludesExplicitRequestRule(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools()[:1],
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{Reply: "Thanks."},
		},
	}
	api := &recordingOrderAPI{}
	a, _ := newReactAttempt(t, cat, model, api)

	if _, err := a.Run(context.Background(), validInput()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(model.toolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(model.toolCalls))
	}
	sys := model.toolCalls[0].System
	if sys != attempt.ToolCallingSystemPrompt {
		t.Error("system prompt must be ToolCallingSystemPrompt")
	}
	lower := strings.ToLower(sys)
	if !strings.Contains(lower, "explicit") || !strings.Contains(lower, "write") {
		t.Errorf("tool-calling prompt must state write tools need an explicit request, got %q", sys)
	}
}

func TestRun_executorAllowsWriteRegardlessOfStatusValue(t *testing.T) {
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"refund","skill_names":["order_support"],"reason":"refund"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_2",
					Name:      "refund_order",
					Arguments: json.RawMessage(`{"order_id":"1001","reason":"customer requested"}`),
				}},
			},
			{Reply: "Refund rf_in_transit for order 1001 is pending."},
		},
	}
	api := &recordingOrderAPI{
		bodies: []json.RawMessage{
			// Executor must not refuse the write based on this status string.
			json.RawMessage(`{"order_id":"1001","status":"in_transit","carrier":"UPS"}`),
			json.RawMessage(`{"refund_id":"rf_in_transit","status":"pending"}`),
		},
		latency: 15,
	}
	a, _ := newReactAttempt(t, cat, model, api)

	in := validInput()
	in.Content = "Please refund order 1001"
	result, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if len(api.requests) != 2 {
		t.Fatalf("HTTP requests = %#v, want status then refund (no status-based refusal)", api.requests)
	}
	if api.requests[1].Name != "refund_order" {
		t.Errorf("second HTTP = %q", api.requests[1].Name)
	}
	if len(out.UsedTools) != 2 || out.UsedTools[1].Status != "ok" {
		t.Errorf("used_tools = %#v", out.UsedTools)
	}
}
