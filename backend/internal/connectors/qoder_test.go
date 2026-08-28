package connectors

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

func TestQoderSerializeTools_UsesOpenAIFunctionWrapper(t *testing.T) {
	tools := serializeTools([]core.Tool{
		{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	})

	if len(tools) != 1 {
		t.Fatalf("expected 1 serialized tool, got %d", len(tools))
	}

	var got map[string]any
	if err := json.Unmarshal(tools[0], &got); err != nil {
		t.Fatalf("unmarshal serialized tool: %v", err)
	}

	if got["type"] != "function" {
		t.Fatalf("tool type = %v, want function", got["type"])
	}
	fn, ok := got["function"].(map[string]any)
	if !ok {
		t.Fatalf("function missing or wrong type: %v", got["function"])
	}
	if fn["name"] != "read_file" {
		t.Fatalf("tool name = %v, want read_file", fn["name"])
	}
	if fn["description"] != "Read a file" {
		t.Fatalf("tool description = %v, want Read a file", fn["description"])
	}

	schema, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters missing or wrong type: %v", fn["parameters"])
	}
	if schema["type"] != "object" {
		t.Fatalf("parameters.type = %v, want object", schema["type"])
	}
}

func TestQoderSerializeTools_DefaultsInvalidSchema(t *testing.T) {
	tools := serializeTools([]core.Tool{
		{Name: "empty"},
		{Name: "bad", Parameters: json.RawMessage(`not-json`)},
	})

	if len(tools) != 2 {
		t.Fatalf("expected 2 serialized tools, got %d", len(tools))
	}

	for i, raw := range tools {
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal serialized tool %d: %v", i, err)
		}
		fn, ok := got["function"].(map[string]any)
		if !ok {
			t.Fatalf("function missing or wrong type for tool %d: %v", i, got)
		}
		schema, ok := fn["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("parameters missing or wrong type for tool %d: %v", i, got)
		}
		if schema["type"] != "object" {
			t.Fatalf("parameters.type for tool %d = %v, want object", i, schema["type"])
		}
		if _, ok := schema["properties"].(map[string]any); !ok {
			t.Fatalf("parameters.properties missing or wrong type for tool %d: %v", i, schema)
		}
	}
}

func TestNormalizeQoderMessages_PreservesToolPair(t *testing.T) {
	msgs, system := normalizeQoderMessages([]core.Message{
		{
			Role: core.RoleAssistant,
			Content: []core.ContentPart{
				{Type: core.PartText, Text: "checking"},
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{
					ID:        "tool_call_1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"a.go"}`),
				}},
			},
		},
		{
			Role: core.RoleUser,
			Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{
					CallID: "tool_call_1", Content: "package main",
				}},
				{Type: core.PartText, Text: "continue"},
			},
		},
	})

	if system != "" {
		t.Fatalf("unexpected system text %q", system)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected assistant/tool/user messages, got %+v", msgs)
	}
	if len(msgs[0].ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", msgs[0])
	}
	call := msgs[0].ToolCalls[0]
	if call.ID != "tool_call_1" || call.Function.Name != "read_file" ||
		call.Function.Arguments != `{"path":"a.go"}` {
		t.Fatalf("unexpected tool call: %+v", call)
	}
	if msgs[1].Role != "tool" || msgs[1].ToolCallID != "tool_call_1" ||
		msgs[1].Content != "package main" {
		t.Fatalf("unexpected tool result: %+v", msgs[1])
	}
	if msgs[2].Role != "user" || msgs[2].Content != "continue" {
		t.Fatalf("unexpected user message: %+v", msgs[2])
	}
}

func TestNormalizeQoderMessages_DropsOrphanResultAndKeepsText(t *testing.T) {
	msgs, _ := normalizeQoderMessages([]core.Message{
		{
			Role: core.RoleUser,
			Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{
					CallID: "tool_call_1", Content: "stale",
				}},
				{Type: core.PartText, Text: "continue"},
			},
		},
	})

	if len(msgs) != 1 {
		t.Fatalf("expected orphan result to be dropped, got %+v", msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Content != "continue" {
		t.Fatalf("unexpected remaining message: %+v", msgs[0])
	}
}

func TestNormalizeQoderMessages_MovesDelayedResultNextToCall(t *testing.T) {
	msgs, _ := normalizeQoderMessages([]core.Message{
		{
			Role: core.RoleAssistant,
			Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{
					ID: "tool_call_1", Name: "read_file", Arguments: json.RawMessage(`{}`),
				}},
			},
		},
		{
			Role: core.RoleUser,
			Content: []core.ContentPart{
				{Type: core.PartText, Text: "intervening"},
			},
		},
		{
			Role: core.RoleTool,
			Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{
					CallID: "tool_call_1", Content: "result",
				}},
			},
		},
	})

	if len(msgs) != 3 {
		t.Fatalf("expected three repaired messages, got %+v", msgs)
	}
	if msgs[0].Role != "assistant" || msgs[1].Role != "tool" || msgs[2].Role != "user" {
		t.Fatalf("tool result is not adjacent to assistant: %+v", msgs)
	}
	if msgs[1].ToolCallID != "tool_call_1" || msgs[1].Content != "result" {
		t.Fatalf("unexpected moved result: %+v", msgs[1])
	}
}

func TestNormalizeQoderMessages_SynthesizesMissingResult(t *testing.T) {
	msgs, _ := normalizeQoderMessages([]core.Message{
		{
			Role: core.RoleAssistant,
			Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{
					ID: "tool_call_1", Name: "read_file", Arguments: json.RawMessage(`{}`),
				}},
			},
		},
		{
			Role: core.RoleUser,
			Content: []core.ContentPart{
				{Type: core.PartText, Text: "continue"},
			},
		},
	})

	if len(msgs) != 3 || msgs[1].Role != "tool" || msgs[1].ToolCallID != "tool_call_1" {
		t.Fatalf("missing tool result was not repaired: %+v", msgs)
	}
	if msgs[1].Content != "" {
		t.Fatalf("synthetic result content = %q, want empty", msgs[1].Content)
	}
}

func TestQoderBuildPayload_ScopesSessionByAccountAndRequest(t *testing.T) {
	connector := NewQoder("qoder", "")
	req := &core.ChatRequest{
		Model: "ultimate",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "hello"},
			}},
		},
		Metadata: core.RequestMetadata{
			ContextAffinityKey: "conversation-a",
			RequestID:          "request-a",
		},
	}

	first := connector.buildPayload(req, "ultimate", nil, "account-a")
	retry := connector.buildPayload(req, "ultimate", nil, "account-a")
	otherAccount := connector.buildPayload(req, "ultimate", nil, "account-b")

	if first.SessionID != retry.SessionID {
		t.Fatalf("same request should keep session id: %q != %q", first.SessionID, retry.SessionID)
	}
	if first.SessionID == otherAccount.SessionID {
		t.Fatal("different accounts must not share a session id")
	}

	req.Metadata.ContextAffinityKey = ""
	withoutAffinity := connector.buildPayload(req, "ultimate", nil, "account-a")
	req.Metadata.RequestID = "request-b"
	otherRequest := connector.buildPayload(req, "ultimate", nil, "account-a")
	if withoutAffinity.SessionID == otherRequest.SessionID {
		t.Fatal("requests without affinity must not share a session id")
	}
}

func TestCollectQoderChunks_ReassemblesToolCall(t *testing.T) {
	chunks := make(chan core.StreamChunk, 3)
	chunks <- core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID:        "tool_call_1",
			Name:      "Read",
			Arguments: json.RawMessage(`{"file_path":"`),
		},
	}
	chunks <- core.StreamChunk{
		Type:     core.ChunkToolCall,
		Index:    0,
		ToolCall: &core.ToolCall{Arguments: json.RawMessage(`a.go"}`)},
	}
	chunks <- core.StreamChunk{
		Type:         core.ChunkFinish,
		FinishReason: core.FinishToolCalls,
	}
	close(chunks)

	msg, finishReason, _, err := collectQoderChunks(chunks)
	if err != nil {
		t.Fatalf("collect chunks: %v", err)
	}
	if finishReason != core.FinishToolCalls {
		t.Fatalf("finish reason = %q, want %q", finishReason, core.FinishToolCalls)
	}
	if len(msg.Content) != 1 || msg.Content[0].ToolCall == nil {
		t.Fatalf("expected one complete tool call, got %+v", msg.Content)
	}
	call := msg.Content[0].ToolCall
	if call.ID != "tool_call_1" || call.Name != "Read" ||
		string(call.Arguments) != `{"file_path":"a.go"}` {
		t.Fatalf("unexpected assembled tool call: %+v", call)
	}
}

func TestUnwrapQoderSSELineWithError_SurfacesEnvelopeError(t *testing.T) {
	line := `data: {"statusCodeValue":400,"body":"Invalid tool parameters"}`

	inner, ok, err := unwrapQoderSSELineWithError(line, "qoder", "claude-sonnet-4")
	if inner != "" || ok {
		t.Fatalf("expected no inner payload, got inner=%q ok=%v", inner, ok)
	}

	var pe *core.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ProviderError, got %T %v", err, err)
	}
	if pe.Kind != core.ErrBadRequest {
		t.Fatalf("error kind = %v, want %v", pe.Kind, core.ErrBadRequest)
	}
	if pe.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", pe.StatusCode)
	}
	if pe.Message != "Invalid tool parameters" {
		t.Fatalf("message = %q, want Invalid tool parameters", pe.Message)
	}
}

func TestUnwrapQoderSSELineWithError_ClassifiesBillingBlocks(t *testing.T) {
	tests := []struct {
		name string
		body string
		kind core.ErrorKind
	}{
		{name: "quota code", body: `{"code":"112","message":"Quota exhausted"}`, kind: core.ErrQuotaExhausted},
		{name: "pricing URL", body: `{"message":"Upgrade required","pricingUrl":"https://qoder.com/pricing"}`, kind: core.ErrQuotaExhausted},
		{name: "queue throttle", body: `{"code":"10605","message":"Queue limit"}`, kind: core.ErrRateLimit},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := `data: {"statusCodeValue":403,"body":` + strconv.Quote(tc.body) + `}`
			_, ok, err := unwrapQoderSSELineWithError(line, "qoder", "model")
			require.False(t, ok)
			var pe *core.ProviderError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, tc.kind, pe.Kind)
			require.Equal(t, core.FailureScopeAccount, pe.Scope)
		})
	}
}

func TestQoderIsPAT_DetectsAuthMethodAndPrefix(t *testing.T) {
	cases := []struct {
		name  string
		creds core.Credentials
		want  bool
	}{
		{
			name:  "marked pat",
			creds: core.Credentials{APIKey: "pt-abc", Extra: map[string]string{"qoder_auth_method": "pat"}},
			want:  true,
		},
		{
			name:  "pt prefix without marker",
			creds: core.Credentials{APIKey: "pt-abc"},
			want:  true,
		},
		{
			name:  "oauth access token",
			creds: core.Credentials{AccessToken: "dt-xyz", Extra: map[string]string{"user_id": "u1"}},
			want:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := qoderIsPAT(tc.creds); got != tc.want {
				t.Fatalf("qoderIsPAT = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQoderPATToken_PrefersAPIKey(t *testing.T) {
	if got := qoderPATToken(core.Credentials{APIKey: "pt-key", AccessToken: "pt-token"}); got != "pt-key" {
		t.Fatalf("qoderPATToken = %q, want pt-key", got)
	}
	if got := qoderPATToken(core.Credentials{AccessToken: "pt-token"}); got != "pt-token" {
		t.Fatalf("qoderPATToken fallback = %q, want pt-token", got)
	}
}

func TestQoderValidateCreds_PATRequiresToken(t *testing.T) {
	connector := NewQoder("qoder", "")

	// A PAT-marked credential with no token must be rejected as an auth error.
	err := connector.validateCreds(core.Credentials{Extra: map[string]string{"qoder_auth_method": "pat"}})
	var pe *core.ProviderError
	if !errors.As(err, &pe) || pe.Kind != core.ErrAuth {
		t.Fatalf("expected auth ProviderError, got %v", err)
	}

	// A PAT-marked credential with a token passes without OAuth fields.
	if err := connector.validateCreds(core.Credentials{APIKey: "pt-abc", Extra: map[string]string{"qoder_auth_method": "pat"}}); err != nil {
		t.Fatalf("validateCreds(pat) = %v, want nil", err)
	}
}

func TestParseQoderJobToken(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantToken string
		wantTTL   time.Duration
	}{
		{
			// expires_in is milliseconds; the ~24h lifetime is kept minus skew.
			name:      "root token with expires_in millis",
			body:      `{"token":"jt-root","expires_in":86400000}`,
			wantToken: "jt-root",
			wantTTL:   24*time.Hour - qoderJobTokenRefreshSkew,
		},
		{
			name:      "nested data jobToken defaults ttl",
			body:      `{"data":{"jobToken":"jt-nested"}}`,
			wantToken: "jt-nested",
			wantTTL:   qoderJobTokenDefaultTTL,
		},
		{
			name:      "non jt token ignored",
			body:      `{"token":"pt-not-a-job-token"}`,
			wantToken: "",
			wantTTL:   0,
		},
		{
			// A sub-skew lifetime is clamped up to the minimum.
			name:      "tiny expiry clamped to min",
			body:      `{"jt":"jt-small","expires_in":1}`,
			wantToken: "jt-small",
			wantTTL:   qoderJobTokenMinTTL,
		},
		{
			name:      "invalid json",
			body:      `not-json`,
			wantToken: "",
			wantTTL:   0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, ttl := parseQoderJobToken([]byte(tc.body))
			if token != tc.wantToken {
				t.Fatalf("token = %q, want %q", token, tc.wantToken)
			}
			if ttl != tc.wantTTL {
				t.Fatalf("ttl = %v, want %v", ttl, tc.wantTTL)
			}
		})
	}
}

func TestParseQoderJobToken_ExpiresAtTimestamp(t *testing.T) {
	// The RFC3339 expires_at takes precedence over expires_in. The captured
	// real response reports both; the absolute timestamp should win.
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"token":"jt-abc","expires_at":"` + expiresAt + `","expires_in":86400000}`
	token, ttl := parseQoderJobToken([]byte(body))
	if token != "jt-abc" {
		t.Fatalf("token = %q, want jt-abc", token)
	}
	// ~24h minus the refresh skew, allowing a small window for test runtime.
	want := 24*time.Hour - qoderJobTokenRefreshSkew
	if ttl > want || ttl < want-time.Minute {
		t.Fatalf("ttl = %v, want ~%v", ttl, want)
	}
}

func TestQoderQuotaFromStatus(t *testing.T) {
	reset := int64(1786374621453)
	wantReset := time.UnixMilli(reset).UTC().Format(time.RFC3339)

	// Pro Trial with quota:0 and not exceeded — the captured real account. It
	// must report the plan (from userTag) with no misleading usage bar.
	trial := qoderQuotaFromStatus(&qoderUserStatus{
		UserType: "personal_professional_trial", Quota: 0, IsQuotaExceeded: false,
		Plan: "PLAN_TIER_PRO_TRIAL", UserTag: "Pro Trial", NextResetAt: reset,
	})
	if trial.PlanName != "Pro Trial" {
		t.Fatalf("trial plan = %q, want Pro Trial", trial.PlanName)
	}
	if len(trial.Quotas) != 0 {
		t.Fatalf("trial quotas = %d, want 0 (pooled/plan-only)", len(trial.Quotas))
	}
	if trial.Message != "Pro Trial plan · usage not metered by Qoder" {
		t.Fatalf("trial message = %q, want the not-metered note", trial.Message)
	}

	// A finite per-user quota renders a full-remaining bar with an RFC3339 reset.
	metered := qoderQuotaFromStatus(&qoderUserStatus{
		UserType: "personal", Quota: 500, IsQuotaExceeded: false,
		Plan: "PLAN_TIER_PRO", NextResetAt: reset,
	})
	if metered.PlanName != "Pro" {
		t.Fatalf("metered plan = %q, want Pro (prettified from PLAN_TIER_PRO)", metered.PlanName)
	}
	if len(metered.Quotas) != 1 {
		t.Fatalf("metered quotas = %d, want 1", len(metered.Quotas))
	}
	if q := metered.Quotas[0]; q.Limit != 500 || q.Remaining != 500 || q.ResetAt != wantReset {
		t.Fatalf("metered quota = %+v, want limit 500 remaining 500 reset %s", q, wantReset)
	}

	// Exhausted quota reports a 0-remaining bar so routing can skip the account.
	exceeded := qoderQuotaFromStatus(&qoderUserStatus{
		UserType: "personal", Quota: 100, IsQuotaExceeded: true, UserTag: "Pro", NextResetAt: reset,
	})
	if len(exceeded.Quotas) != 1 || exceeded.Quotas[0].Remaining != 0 {
		t.Fatalf("exceeded quota = %+v, want a single 0-remaining entry", exceeded.Quotas)
	}

	// Team seats draw from a pooled org quota; quota:0 there means pooled.
	team := qoderQuotaFromStatus(&qoderUserStatus{
		UserType: "teams", Quota: 0, IsQuotaExceeded: false, UserTag: "Team",
	})
	if len(team.Quotas) != 0 || team.Message != "Team plan · shared team pool" {
		t.Fatalf("team quota = %+v msg=%q, want plan-only shared team pool", team.Quotas, team.Message)
	}
}

func TestPrettifyQoderPlan(t *testing.T) {
	cases := []struct{ plan, tag, want string }{
		{"PLAN_TIER_PRO_TRIAL", "Pro Trial", "Pro Trial"}, // userTag preferred
		{"PLAN_TIER_PRO_TRIAL", "", "Pro Trial"},          // prettified enum
		{"PLAN_TIER_ENTERPRISE", "", "Enterprise"},
		{"", "", "Qoder"}, // fallback
	}
	for _, tc := range cases {
		if got := prettifyQoderPlan(tc.plan, tc.tag); got != tc.want {
			t.Errorf("prettifyQoderPlan(%q, %q) = %q, want %q", tc.plan, tc.tag, got, tc.want)
		}
	}
}
