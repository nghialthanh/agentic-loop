package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agentic-loop/worker/internal/attempt"
)

// OpenAI is an OpenAI-compatible chat completions client (temperature 0).
type OpenAI struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
	Tools       []openaiTool  `json:"tools,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type openaiTool struct {
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolCallFn   `json:"function"`
}

type toolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// SelectSkills runs the skill-selection call (no tool schemas).
func (c *OpenAI) SelectSkills(ctx context.Context, call attempt.SkillSelectionCall) (string, error) {
	msgs := []chatMessage{
		{Role: "system", Content: call.System},
		{Role: "user", Content: call.Customer},
	}
	return c.completeContent(ctx, msgs, nil)
}

// CallWithTools runs the tool-calling / reply call.
func (c *OpenAI) CallWithTools(ctx context.Context, call attempt.ToolCallingCall) (attempt.ToolCallingResult, error) {
	msgs := make([]chatMessage, 0, len(call.Messages)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: call.System})
	for _, m := range call.Messages {
		role := m.Role
		switch role {
		case "customer":
			role = "user"
		case "agent":
			role = "assistant"
		case "tool":
			msgs = append(msgs, chatMessage{Role: "tool", Name: m.Name, Content: m.Content})
			continue
		}
		msgs = append(msgs, chatMessage{Role: role, Content: m.Content, Name: m.Name})
	}
	tools := make([]openaiTool, 0, len(call.Tools))
	for _, t := range call.Tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{}`)
		}
		tools = append(tools, openaiTool{
			Type: "function",
			Function: openaiToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	raw, usage, err := c.complete(ctx, msgs, tools)
	if err != nil {
		return attempt.ToolCallingResult{}, err
	}
	if len(raw.Choices) == 0 {
		return attempt.ToolCallingResult{}, fmt.Errorf("empty model choices")
	}
	msg := raw.Choices[0].Message
	out := attempt.ToolCallingResult{
		Reply:            msg.Content,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
	}
	for _, tc := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, attempt.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	return out, nil
}

func (c *OpenAI) completeContent(ctx context.Context, msgs []chatMessage, tools []openaiTool) (string, error) {
	raw, _, err := c.complete(ctx, msgs, tools)
	if err != nil {
		return "", err
	}
	if len(raw.Choices) == 0 {
		return "", fmt.Errorf("empty model choices")
	}
	return raw.Choices[0].Message.Content, nil
}

func (c *OpenAI) complete(ctx context.Context, msgs []chatMessage, tools []openaiTool) (chatResponse, struct {
	PromptTokens     int
	CompletionTokens int
}, error) {
	var zero struct {
		PromptTokens     int
		CompletionTokens int
	}
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return chatResponse{}, zero, fmt.Errorf("MODEL_BASE_URL is required")
	}
	modelName := strings.TrimSpace(c.Model)
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}
	payload := chatRequest{
		Model:       modelName,
		Temperature: 0,
		Messages:    msgs,
		Tools:       tools,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return chatResponse{}, zero, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatResponse{}, zero, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(c.APIKey); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(httpReq)
	if err != nil {
		return chatResponse{}, zero, err
	}
	defer res.Body.Close()
	rawBody, err := io.ReadAll(res.Body)
	if err != nil {
		return chatResponse{}, zero, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return chatResponse{}, zero, fmt.Errorf("model HTTP %d: %s", res.StatusCode, truncate(string(rawBody), 200))
	}
	var parsed chatResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return chatResponse{}, zero, fmt.Errorf("decode model response: %w", err)
	}
	return parsed, struct {
		PromptTokens     int
		CompletionTokens int
	}{parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
