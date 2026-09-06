package transform

import (
	"bytes"
	"fmt"
	"strings"

	json "github.com/mydisha/keirouter/backend/internal/fastjson"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// AnthropicCodec handles the Anthropic Messages wire format (/v1/messages).
type AnthropicCodec struct{}

func (AnthropicCodec) Dialect() core.Dialect { return core.DialectAnthropic }

// ---- wire types -------------------------------------------------------------

type antRequest struct {
	Model      string          `json:"model"`
	System     json.RawMessage `json:"system,omitempty"`
	Messages   []antMessage    `json:"messages"`
	Tools      []antTool       `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	MaxTokens  int             `json:"max_tokens"`
	Stream     bool            `json:"stream,omitempty"`
	Temp       *float64        `json:"temperature,omitempty"`
	TopP       *float64        `json:"top_p,omitempty"`
	Stop       []string        `json:"stop_sequences,omitempty"`
	// Thinking carries the extended-thinking configuration that clients like
	// Claude Code send. It must be forwarded to Anthropic-compatible upstreams
	// (e.g. GLM, Zhipu) or the model will not emit reasoning blocks, confusing
	// clients that expect them.
	Thinking json.RawMessage `json:"thinking,omitempty"`
}

type antMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type antBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Thinking holds the reasoning content for thinking blocks. Anthropic uses
	// "thinking" as the JSON key (not "text") for this block type.
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Source    *antImageSource `json:"source,omitempty"`
	// Signature is the cryptographic proof tag for thinking blocks that must be
	// echoed back to the upstream on the next turn. Only the originating provider's
	// signatures are valid; foreign ones (from combo-mixed models) are rejected.
	Signature string `json:"signature,omitempty"`
}

type antImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type antTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// ---- request parsing --------------------------------------------------------

func (AnthropicCodec) ParseRequest(body []byte) (*core.ChatRequest, error) {
	var raw antRequest
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anthropic: parse request: %w", err)
	}

	maxTokens := raw.MaxTokens
	req := &core.ChatRequest{
		Model:       raw.Model,
		System:      decodeAntSystem(raw.System),
		Temperature: raw.Temp,
		TopP:        raw.TopP,
		Stop:        raw.Stop,
		Stream:      raw.Stream,
	}
	if maxTokens > 0 {
		req.MaxTokens = &maxTokens
	}

	for _, t := range raw.Tools {
		req.Tools = append(req.Tools, core.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	req.ToolChoice = claudeToolChoiceToOpenAI(raw.ToolChoice)

	for _, m := range raw.Messages {
		req.Messages = append(req.Messages, parseAntMessage(m))
	}

	// Parse thinking configuration from the raw body (before unmarshaling)
	req.Reasoning = parseAntThinkingFromBytes(body)

	return req, nil
}

// parseAntThinkingFromBytes extracts thinking configuration from raw JSON bytes.
// This is called before the antRequest struct is unmarshaled to capture the
// thinking field which is not part of the antRequest struct.
func parseAntThinkingFromBytes(body []byte) *core.ReasoningConfig {
	var thinkingWrapper struct {
		Thinking *struct {
			Type         string `json:"type"`
			BudgetTokens int    `json:"budget_tokens,omitempty"`
		} `json:"thinking"`
	}
	if err := json.Unmarshal(body, &thinkingWrapper); err == nil && thinkingWrapper.Thinking != nil {
		cfg := &core.ReasoningConfig{}
		switch thinkingWrapper.Thinking.Type {
		case "enabled":
			cfg.Effort = "high"
			if thinkingWrapper.Thinking.BudgetTokens > 0 {
				cfg.MaxTokens = thinkingWrapper.Thinking.BudgetTokens
			}
		case "adaptive":
			cfg.Effort = "adaptive"
		}
		return cfg
	}
	return nil
}

func decodeAntSystem(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []antBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var out string
	for _, b := range blocks {
		if b.Type == "text" {
			out += b.Text
		}
	}
	return out
}

func parseAntMessage(m antMessage) core.Message {
	msg := core.Message{Role: mapAntRole(m.Role)}

	// Content may be a plain string or an array of blocks.
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: s})
		return msg
	}

	var blocks []antBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return msg
	}
	for _, b := range blocks {
		part, ok := parseAntContentBlock(b)
		if ok {
			msg.Content = append(msg.Content, part)
		}
	}
	return msg
}

func parseAntContentBlock(block antBlock) (core.ContentPart, bool) {
	switch block.Type {
	case "text":
		return parseAntTextBlock(block), true
	case "thinking":
		return parseAntThinkingContentBlock(block), true
	case "tool_use":
		return parseAntToolUseBlock(block), true
	case "tool_result":
		return parseAntToolResultBlock(block), true
	case "image":
		return parseAntImageContentBlock(block)
	default:
		return core.ContentPart{}, false
	}
}

func parseAntTextBlock(block antBlock) core.ContentPart {
	return core.ContentPart{Type: core.PartText, Text: block.Text}
}

func parseAntThinkingContentBlock(block antBlock) core.ContentPart {
	return core.ContentPart{
		Type:      core.PartThinking,
		Text:      block.Thinking,
		Signature: block.Signature,
	}
}

func parseAntToolUseBlock(block antBlock) core.ContentPart {
	return core.ContentPart{
		Type:     core.PartToolCall,
		ToolCall: &core.ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Input},
	}
}

func parseAntToolResultBlock(block antBlock) core.ContentPart {
	return core.ContentPart{
		Type: core.PartToolResult,
		ToolResult: &core.ToolResult{
			CallID:  block.ToolUseID,
			Content: decodeAntToolResultContent(block.Content),
			IsError: block.IsError,
		},
	}
}

func parseAntImageContentBlock(block antBlock) (core.ContentPart, bool) {
	if block.Source == nil {
		return core.ContentPart{}, false
	}
	if block.Source.Type == "url" && block.Source.URL != "" {
		return core.ContentPart{Type: core.PartImage, Media: &core.MediaPayload{URL: block.Source.URL}}, true
	}
	return core.ContentPart{
		Type:  core.PartImage,
		Media: &core.MediaPayload{MIMEType: block.Source.MediaType, Data: block.Source.Data},
	}, true
}

func decodeAntToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []antBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return string(raw)
	}
	var out string
	for _, b := range blocks {
		if b.Type == "text" {
			out += b.Text
		}
	}
	return out
}

func mapAntRole(role string) core.Role {
	switch role {
	case "assistant":
		return core.RoleAssistant
	default:
		return core.RoleUser
	}
}

// ---- request rendering ------------------------------------------------------

// antDefaultMaxOutput is the conservative output ceiling for Claude models.
const antDefaultMaxOutput = 64000

func (AnthropicCodec) RenderRequest(req *core.ChatRequest) ([]byte, error) {
	maxTokens, thinkingBudget := antTokenLimits(req)
	out := antRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Stream:    req.Stream,
		Temp:      req.Temperature,
		TopP:      req.TopP,
		Stop:      req.Stop,
	}
	renderAntRequestMetadata(&out, req, thinkingBudget)
	out.Messages = renderAntMessages(req.Messages)

	return json.Marshal(out)
}

func antTokenLimits(req *core.ChatRequest) (int, int) {
	maxTokens := 4096
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}
	if req.Reasoning == nil || req.Reasoning.MaxTokens <= 0 {
		return maxTokens, 0
	}

	thinkingBudget := req.Reasoning.MaxTokens
	if thinkingBudget < maxTokens {
		return maxTokens, thinkingBudget
	}
	maxTokens = min(thinkingBudget+1024, antDefaultMaxOutput)
	if thinkingBudget >= maxTokens {
		thinkingBudget = max(1024, maxTokens-1024)
	}
	return maxTokens, thinkingBudget
}

func renderAntRequestMetadata(out *antRequest, req *core.ChatRequest, thinkingBudget int) {
	if modelRejectsTemperature(req.Model) {
		out.Temp = nil
	}
	out.Thinking = renderAntThinking(req.Reasoning, thinkingBudget)
	if req.System != "" {
		out.System, _ = json.Marshal(req.System)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, antTool{Name: t.Name, Description: t.Description, InputSchema: t.Parameters})
	}
	if len(out.Tools) > 0 {
		if tc := openAIToolChoiceToClaude(req.ToolChoice); tc != nil {
			out.ToolChoice, _ = json.Marshal(tc)
		}
	}
}

// renderAntThinking forwards extended-thinking configuration to compatible upstreams.
func renderAntThinking(reasoning *core.ReasoningConfig, budget int) json.RawMessage {
	if reasoning == nil {
		return nil
	}
	thinkingType := antThinkingType(reasoning.Effort, budget)
	if thinkingType == "" {
		return nil
	}
	thinking := map[string]any{"type": thinkingType}
	if thinkingType == "enabled" && budget > 0 {
		thinking["budget_tokens"] = budget
	}
	raw, _ := json.Marshal(thinking)
	return raw
}

func antThinkingType(effort string, budget int) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "adaptive", "auto":
		return "adaptive"
	case "", "none", "off", "disabled":
		if budget == 0 {
			return ""
		}
	}
	return "enabled"
}

func renderAntMessages(messages []core.Message) []antMessage {
	var messagesOut []antMessage
	for _, message := range messages {
		blocks := renderAntBlocks(message)
		raw, _ := json.Marshal(blocks)
		role := "user"
		if message.Role == core.RoleAssistant {
			role = "assistant"
		}
		messagesOut = appendAntMessage(messagesOut, role, raw, blocks)
	}
	return messagesOut
}

func renderAntBlocks(m core.Message) []antBlock {
	var blocks []antBlock
	for _, p := range m.Content {
		blocks = append(blocks, renderAntBlock(p)...)
	}
	if len(blocks) == 0 {
		blocks = append(blocks, antBlock{Type: "text", Text: " "})
	}
	return blocks
}

func renderAntBlock(part core.ContentPart) []antBlock {
	switch part.Type {
	case core.PartText:
		if part.Text != "" {
			return []antBlock{{Type: "text", Text: part.Text}}
		}
	case core.PartThinking:
		return []antBlock{renderAntThinkingBlock(part)}
	case core.PartToolCall:
		return []antBlock{{Type: "tool_use", ID: part.ToolCall.ID, Name: part.ToolCall.Name, Input: normalizeAntToolInputRaw(part.ToolCall.Arguments)}}
	case core.PartToolResult:
		content, _ := json.Marshal(part.ToolResult.Content)
		return []antBlock{{Type: "tool_result", ToolUseID: part.ToolResult.CallID, Content: content, IsError: part.ToolResult.IsError}}
	case core.PartImage:
		return renderAntImageBlock(part.Media)
	}
	return nil
}

func renderAntThinkingBlock(part core.ContentPart) antBlock {
	return antBlock{Type: "thinking", Thinking: part.Text, Signature: part.Signature}
}

func renderAntImageBlock(media *core.MediaPayload) []antBlock {
	if media == nil {
		return nil
	}
	if media.Data != "" {
		return []antBlock{{Type: "image", Source: &antImageSource{Type: "base64", MediaType: media.MIMEType, Data: media.Data}}}
	}
	if media.URL != "" {
		return []antBlock{{Type: "image", Source: &antImageSource{Type: "url", URL: media.URL}}}
	}
	return nil
}

// appendAntMessage merges blocks into the previous message when roles match, as
// Anthropic forbids consecutive messages with the same role.
func appendAntMessage(msgs []antMessage, role string, raw json.RawMessage, blocks []antBlock) []antMessage {
	if n := len(msgs); n > 0 && msgs[n-1].Role == role {
		var prev []antBlock
		_ = json.Unmarshal(msgs[n-1].Content, &prev)
		prev = append(prev, blocks...)
		merged, _ := json.Marshal(prev)
		msgs[n-1].Content = merged
		return msgs
	}
	return append(msgs, antMessage{Role: role, Content: raw})
}

func normalizeAntToolInputRaw(raw json.RawMessage) json.RawMessage {
	if antToolInputIsObject(raw) {
		return raw
	}
	return json.RawMessage(`{}`)
}

func normalizeAntToolInputValue(raw json.RawMessage) any {
	raw = normalizeAntToolInputRaw(raw)
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		return map[string]any{}
	}
	return input
}

func antToolInputIsObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	return raw[0] == '{'
}

// ---- response parsing -------------------------------------------------------

type antResponse struct {
	ID         string     `json:"id"`
	Model      string     `json:"model"`
	Content    []antBlock `json:"content"`
	StopReason string     `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func (AnthropicCodec) ParseResponse(body []byte, model string) (*core.ChatResponse, error) {
	var raw antResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}

	msg := core.Message{Role: core.RoleAssistant}
	for _, b := range raw.Content {
		switch b.Type {
		case "text":
			msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: b.Text})
		case "thinking":
			msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: b.Thinking})
		case "tool_use":
			msg.Content = append(msg.Content, core.ContentPart{
				Type:     core.PartToolCall,
				ToolCall: &core.ToolCall{ID: b.ID, Name: b.Name, Arguments: b.Input},
			})
		}
	}

	return &core.ChatResponse{
		ID:           raw.ID,
		Model:        firstNonEmpty(raw.Model, model),
		Message:      msg,
		FinishReason: mapAntStop(raw.StopReason),
		Usage: core.Usage{
			PromptTokens:     raw.Usage.InputTokens + raw.Usage.CacheReadInputTokens + raw.Usage.CacheCreationInputTokens,
			CompletionTokens: raw.Usage.OutputTokens,
			TotalTokens: raw.Usage.InputTokens + raw.Usage.CacheReadInputTokens +
				raw.Usage.CacheCreationInputTokens + raw.Usage.OutputTokens,
			CachedTokens:     raw.Usage.CacheReadInputTokens,
			CacheWriteTokens: raw.Usage.CacheCreationInputTokens,
			Source:           core.UsageSourceProvider,
		},
	}, nil
}

func (AnthropicCodec) RenderResponse(resp *core.ChatResponse) ([]byte, error) {
	var content []map[string]any
	for _, p := range resp.Message.Content {
		switch p.Type {
		case core.PartText:
			content = append(content, map[string]any{"type": "text", "text": p.Text})
		case core.PartThinking:
			content = append(content, map[string]any{"type": "thinking", "thinking": p.Text})
		case core.PartToolCall:
			content = append(content, map[string]any{
				"type": "tool_use", "id": p.ToolCall.ID, "name": p.ToolCall.Name, "input": normalizeAntToolInputValue(p.ToolCall.Arguments),
			})
		}
	}
	out := map[string]any{
		"id":          firstNonEmpty(resp.ID, "msg_"+resp.Model),
		"type":        "message",
		"role":        "assistant",
		"model":       resp.Model,
		"content":     content,
		"stop_reason": renderAntStop(resp.FinishReason),
		"usage": map[string]int{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
		},
	}
	return json.Marshal(out)
}

func mapAntStop(r string) core.FinishReason {
	switch r {
	case "end_turn", "stop_sequence":
		return core.FinishStop
	case "max_tokens":
		return core.FinishLength
	case "tool_use":
		return core.FinishToolCalls
	default:
		return core.FinishStop
	}
}

func renderAntStop(r core.FinishReason) string {
	switch r {
	case core.FinishLength:
		return "max_tokens"
	case core.FinishToolCalls:
		return "tool_use"
	default:
		return "end_turn"
	}
}
