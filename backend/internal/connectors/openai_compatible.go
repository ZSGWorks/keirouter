package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/transform"
)

const (
	codeBuddyUserAgent   = "CLI/2.108.1 CodeBuddy/2.108.1"
	agentRouterUserAgent = "RooCode/3.54.0"
	kimchiUserAgent      = "kimchi/0.1.50"
)

// OpenAICompatible drives any endpoint that speaks the OpenAI Chat Completions
// API: OpenAI itself, plus GLM, MiniMax, DeepSeek, Groq, Together, and custom
// gateways. The provider id and default base URL are supplied at construction
// so one implementation backs many registered providers.
type OpenAICompatible struct {
	id          string
	defaultBase string
	codec       transform.OpenAICodec
}

type renderedChatRequest struct {
	req  *core.ChatRequest
	body []byte
	url  string
}

// NewOpenAICompatible builds a connector for an OpenAI-compatible provider.
func NewOpenAICompatible(id, defaultBaseURL string) *OpenAICompatible {
	return &OpenAICompatible{id: id, defaultBase: defaultBaseURL}
}

func (c *OpenAICompatible) ID() string            { return c.id }
func (c *OpenAICompatible) Dialect() core.Dialect { return core.DialectOpenAI }

func (c *OpenAICompatible) baseURL(creds core.Credentials) string {
	return resolvedBaseURL(c.defaultBase, creds)
}

func resolvedBaseURL(defaultBase string, creds core.Credentials) string {
	u := defaultBase
	if creds.BaseURL != "" {
		u = creds.BaseURL
	}
	// Resolve template placeholders like {accountId} from creds.Extra.
	// Cloudflare Workers AI uses: /accounts/{accountId}/ai/v1/chat/completions
	for key, val := range creds.Extra {
		u = strings.ReplaceAll(u, "{"+key+"}", val)
	}
	return u
}

func (c *OpenAICompatible) headers(creds core.Credentials) map[string]string {
	return mergeHeaders(c.providerHeaders(creds), creds.Headers)
}

func (c *OpenAICompatible) providerHeaders(creds core.Credentials) map[string]string {
	if c.id == "azure" {
		return azureHeaders(creds)
	}

	// Cline requires a workos: prefix on the access token and custom headers.
	if c.id == "cline" {
		return clineHeaders(creds)
	}

	// CodeBuddy requires CLI headers on every request.
	if c.id == "codebuddy" {
		return codeBuddyHeaders(creds)
	}

	// AgentRouter restricts upstream access to a known set of CLI/IDE tools
	// (Claude Code CLI, Cline, Roo Code, Kilo Code, Qwen Code, OpenCode, etc.).
	// Use a Roo Code-compatible fingerprint so outbound requests pass the
	// endpoint allowlist. Auth mirrors the default Bearer path below.
	if c.id == "agentrouter" {
		return agentRouterHeaders(creds)
	}

	// Kimchi requires a custom User-Agent.
	if c.id == "kimchi" {
		return kimchiHeaders(creds)
	}

	return authorizationHeaders(creds)
}

func azureHeaders(creds core.Credentials) map[string]string {
	h := map[string]string{}
	if creds.AccessToken != "" {
		h["Authorization"] = bearer(creds.AccessToken)
	} else if creds.APIKey != "" {
		h["api-key"] = creds.APIKey
	}
	if org := creds.Extra["organization"]; org != "" {
		h["OpenAI-Organization"] = org
	}
	return h
}

func clineHeaders(creds core.Credentials) map[string]string {
	token := credentialToken(creds)
	if token != "" && !strings.HasPrefix(token, "workos:") {
		token = "workos:" + token
	}
	return map[string]string{
		"Authorization":  bearer(token),
		"HTTP-Referer":   "https://cline.bot",
		"X-Title":        "Cline",
		"X-CLIENT-TYPE":  "keirouter",
		"X-PLATFORM":     "unknown",
		"X-IS-MULTIROOT": "false",
	}
}

func codeBuddyHeaders(creds core.Credentials) map[string]string {
	h := authorizationHeaders(creds)
	h["User-Agent"] = codeBuddyUserAgent
	h["X-Product"] = "SaaS"
	h["X-IDE-Type"] = "CLI"
	h["X-IDE-Name"] = "CLI"
	h["X-Requested-With"] = "XMLHttpRequest"
	h["x-codebuddy-request"] = "1"
	return h
}

func agentRouterHeaders(creds core.Credentials) map[string]string {
	h := authorizationHeaders(creds)
	h["HTTP-Referer"] = "https://github.com/RooVetGit/Roo-Cline"
	h["X-Title"] = "Roo Code"
	h["User-Agent"] = agentRouterUserAgent
	h["X-Stainless-Arch"] = "x64"
	h["X-Stainless-Lang"] = "js"
	h["X-Stainless-OS"] = "Windows"
	h["X-Stainless-Package-Version"] = "5.12.2"
	h["X-Stainless-Retry-Count"] = "0"
	h["X-Stainless-Runtime"] = "node"
	h["X-Stainless-Runtime-Version"] = "v24.14.0"
	return h
}

func kimchiHeaders(creds core.Credentials) map[string]string {
	h := authorizationHeaders(creds)
	h["User-Agent"] = kimchiUserAgent
	return h
}

func authorizationHeaders(creds core.Credentials) map[string]string {
	if token := credentialToken(creds); token != "" {
		return map[string]string{"Authorization": bearer(token)}
	}
	return map[string]string{}
}

func credentialToken(creds core.Credentials) string {
	if creds.AccessToken != "" {
		return creds.AccessToken
	}
	return creds.APIKey
}

func (c *OpenAICompatible) chatCompletionsURL(creds core.Credentials, model string) string {
	if c.id == "azure" {
		endpoint := strings.TrimRight(creds.Extra["azure_endpoint"], "/")
		if endpoint == "" {
			endpoint = strings.TrimRight(creds.BaseURL, "/")
		}
		deployment := creds.Extra["deployment"]
		if deployment == "" {
			deployment = model
		}
		if deployment == "" {
			deployment = "gpt-4"
		}
		apiVersion := creds.Extra["api_version"]
		if apiVersion == "" {
			apiVersion = "2024-10-01-preview"
		}
		return endpoint + "/openai/deployments/" + url.PathEscape(deployment) +
			"/chat/completions?api-version=" + url.QueryEscape(apiVersion)
	}
	return joinURL(c.baseURL(creds), "chat/completions")
}

// providerRequiresStreaming reports whether the given provider only accepts
// streaming requests. Some OpenAI-compatible providers reject non-streaming
// requests with "Stream must be set to true" (HTTP 400). For these providers,
// the Chat method must use the Stream method internally and drain the chunks
// into a single response.
func providerRequiresStreaming(providerID string) bool {
	switch providerID {
	// Add providers here that require streaming-only requests.
	// These providers return 400 "Stream must be set to true" when
	// a non-streaming request is sent.
	case "xiaomi-mimo", "xiaomi-tokenplan", "mimo-free", "codebuddy":
		return true
	default:
		return false
	}
}

// isStreamRequiredError reports whether the error indicates the upstream
// provider rejected a non-streaming request with "Stream must be set to true".
// This is used for auto-retry with streaming when an unknown (custom) OpenAI-
// compatible provider requires streaming.
func isStreamRequiredError(err error) bool {
	if err == nil {
		return false
	}
	pe := core.AsProviderError(err)
	if pe == nil || pe.StatusCode != http.StatusBadRequest {
		return false
	}
	msg := strings.ToLower(pe.Message)
	return strings.Contains(msg, "stream must be set to true") ||
		strings.Contains(msg, "streaming is required") ||
		strings.Contains(msg, "stream parameter is required")
}

// drainStreamToResponse consumes a stream channel and folds the chunks into a
// single ChatResponse. Used by Chat when the provider requires streaming.
func drainStreamToResponse(stream <-chan core.StreamChunk, model string) (*core.ChatResponse, error) {
	accumulator := streamResponseAccumulator{
		toolCalls: map[string]*core.ToolCall{},
		finish:    core.FinishStop,
	}
	for ch := range stream {
		if err := accumulator.add(ch); err != nil {
			return nil, err
		}
	}
	return accumulator.response(model), nil
}

type streamResponseAccumulator struct {
	text, thinking string
	toolCalls      map[string]*core.ToolCall
	toolOrder      []string
	finish         core.FinishReason
	usage          core.Usage
}

func (a *streamResponseAccumulator) add(ch core.StreamChunk) error {
	if ch.Type == core.ChunkError {
		return ch.Err
	}
	switch ch.Type {
	case core.ChunkText:
		a.text += ch.Delta
	case core.ChunkThinking:
		a.thinking += ch.Delta
	default:
		a.addMetadata(ch)
	}
	return nil
}

func (a *streamResponseAccumulator) addMetadata(ch core.StreamChunk) {
	switch ch.Type {
	case core.ChunkToolCall:
		a.addToolCall(ch.ToolCall)
	case core.ChunkFinish:
		if ch.FinishReason != "" {
			a.finish = ch.FinishReason
		}
	case core.ChunkUsage:
		if ch.Usage != nil {
			a.usage = *ch.Usage
		}
	}
}

func (a *streamResponseAccumulator) addToolCall(call *core.ToolCall) {
	if call == nil {
		return
	}
	existing, ok := a.toolCalls[call.ID]
	if !ok {
		copy := *call
		a.toolCalls[call.ID] = &copy
		a.toolOrder = append(a.toolOrder, call.ID)
	} else if len(call.Arguments) > 0 {
		existing.Arguments = append(existing.Arguments, call.Arguments...)
	}
	a.finish = core.FinishToolCalls
}

func (a *streamResponseAccumulator) response(model string) *core.ChatResponse {
	msg := core.Message{Role: core.RoleAssistant}
	if a.thinking != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: a.thinking})
	}
	if a.text != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: a.text})
	}
	for _, id := range a.toolOrder {
		tc := a.toolCalls[id]
		if len(tc.Arguments) == 0 {
			tc.Arguments = json.RawMessage("{}")
		}
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartToolCall, ToolCall: tc})
	}
	return &core.ChatResponse{Model: model, Message: msg, FinishReason: a.finish, Usage: a.usage}
}

func (c *OpenAICompatible) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	// Some OpenAI-compatible providers only accept streaming requests and reject
	// non-streaming requests with 400 "Stream must be set to true". For known
	// stream-only providers, use the Stream method directly. For unknown
	// providers (including custom OpenAI-compatible ones), the initial non-
	// streaming call may fail with this error — we detect it and retry with
	// streaming transparently.
	if providerRequiresStreaming(c.id) {
		return c.chatFromStream(ctx, req, creds)
	}

	req.Stream = false
	body, err := c.codec.RenderRequestForProvider(req, c.id)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	rendered := renderedChatRequest{
		req:  req,
		body: body,
		url:  c.chatCompletionsURL(creds, req.Model),
	}
	if sc, ok := interface{}(c.codec).(transform.StreamingResponseCodec); ok {
		return c.chatFromStreamingResponse(ctx, creds, rendered, sc)
	}
	return c.chatFromBufferedResponse(ctx, creds, rendered)
}

func (c *OpenAICompatible) chatFromStream(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	stream, err := c.Stream(ctx, req, creds, core.StreamConfig{})
	if err != nil {
		return nil, err
	}
	return drainStreamToResponse(stream, req.Model)
}

// Use streaming JSON decode when the codec supports it — avoids buffering
// the entire response body into a []byte before parsing.
func (c *OpenAICompatible) chatFromStreamingResponse(ctx context.Context, creds core.Credentials, rendered renderedChatRequest, codec transform.StreamingResponseCodec) (*core.ChatResponse, error) {
	_, respBody, err := doJSONDecode(ctx, c.id, rendered.req.Model, rendered.url, rendered.body, c.headers(creds))
	if err != nil {
		return c.chatWithStreamFallback(ctx, rendered.req, creds, err)
	}
	defer respBody.Close()
	resp, err := codec.ParseResponseFrom(respBody, rendered.req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: rendered.req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

func (c *OpenAICompatible) chatFromBufferedResponse(ctx context.Context, creds core.Credentials, rendered renderedChatRequest) (*core.ChatResponse, error) {
	respBody, err := doJSON(ctx, c.id, rendered.req.Model, rendered.url, rendered.body, c.headers(creds))
	if err != nil {
		return c.chatWithStreamFallback(ctx, rendered.req, creds, err)
	}
	resp, err := c.codec.ParseResponse(respBody, rendered.req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: rendered.req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

func (c *OpenAICompatible) chatWithStreamFallback(ctx context.Context, req *core.ChatRequest, creds core.Credentials, err error) (*core.ChatResponse, error) {
	if !isStreamRequiredError(err) {
		return nil, err
	}
	return c.chatFromStream(ctx, req, creds)
}

// Validate probes the upstream /models endpoint to confirm the credentials are
// accepted. Returns nil on success.
func (c *OpenAICompatible) Validate(ctx context.Context, creds core.Credentials) error {
	if c.id == "azure" {
		return c.validateAzure(ctx, creds)
	}

	url := joinURL(c.baseURL(creds), "models")
	_, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", url, nil, c.headers(creds))
	// An HTML response means the base URL points at a web frontend, not the API
	// (e.g. a custom base URL missing the "/v1" path). Fail hard rather than
	// falling through to the chat probe, which is skipped for custom providers
	// and would otherwise report a false-positive success.
	if isNonJSONResponseError(err) {
		return c.validationError(err)
	}
	if err == nil {
		return c.validateModelsSuccess(ctx, creds)
	}
	return c.validateModelsError(ctx, creds, err)
}

func (c *OpenAICompatible) validateAzure(ctx context.Context, creds core.Credentials) error {
	body := []byte(`{"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`)
	err := validateProbe(ctx, validationProbe{c.id, c.chatCompletionsURL(creds, "validate"), body, c.headers(creds)})
	if err != nil {
		return c.validationError(err)
	}
	return nil
}

func (c *OpenAICompatible) validateModelsSuccess(ctx context.Context, creds core.Credentials) error {
	// GET /models reaches local and strict providers conclusively. Other keyed
	// providers may expose models publicly, so verify credentials with chat.
	if !hasCredentials(creds) || strictModelsValidation(c.id) {
		return nil
	}
	if err := c.chatAuthProbe(ctx, creds); err != nil {
		return c.validationError(err)
	}
	return nil
}

func (c *OpenAICompatible) validateModelsError(ctx context.Context, creds core.Credentials, err error) error {
	if c.id == "xai" {
		pe := core.AsProviderError(err)
		if pe.StatusCode == http.StatusForbidden {
			return nil
		}
		return c.validationError(err)
	}
	if strictModelsValidation(c.id) {
		return c.validationError(err)
	}
	if validationAuthError(err) || !validationReachedUpstream(err) {
		return c.validationError(err)
	}

	// Many OpenAI-compatible providers either omit /models or reject unknown
	// probe models with 400/404 while still accepting the credential. Fall back
	// to a minimal chat request and treat any non-auth HTTP response as proof
	// that the connection reached the provider.
	if err := c.chatAuthProbe(ctx, creds); err != nil {
		return c.validationError(err)
	}
	return nil
}

func (c *OpenAICompatible) validationError(err error) error {
	return fmt.Errorf("validation failed for %s: %w", c.id, err)
}

func hasCredentials(creds core.Credentials) bool {
	return strings.TrimSpace(creds.APIKey) != "" || strings.TrimSpace(creds.AccessToken) != ""
}

// chatAuthProbe issues a minimal, near-zero-cost chat request (max_tokens=1) to
// confirm the credential is actually accepted by the upstream. It relies on
// validateProbe semantics: only an auth failure (401/403) or a transport error
// that never reached the provider counts as a failure. A non-auth HTTP response
// (e.g. a 400/404 for an unknown probe model) still proves the key was accepted.
func (c *OpenAICompatible) chatAuthProbe(ctx context.Context, creds core.Credentials) error {
	probeModel := ProbeModelForCredentials(ctx, c.id, creds, &OpenAICompatibleModelSource{
		provider: c.id, defaultBase: c.baseURL(creds),
	})
	// Sending an invented model id can trigger upstream model_not_allowed before
	// credential checks. Without a discovered model, rely on the GET /models
	// probe instead of issuing a model-bound request.
	if probeModel == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"model": probeModel,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
		"stream":     false,
	})
	return validateProbe(ctx, validationProbe{c.id, c.chatCompletionsURL(creds, probeModel), body, c.headers(creds)})
}

type validationProbe struct {
	provider string
	endpoint string
	body     []byte
	headers  map[string]string
}

func validateProbe(ctx context.Context, probe validationProbe) error {
	_, err := doJSON(ctx, probe.provider, "validate", probe.endpoint, probe.body, probe.headers)
	if probeFailureIsFatal(err) {
		return err
	}
	return nil
}

func probeFailureIsFatal(err error) bool {
	return err != nil && (isNonJSONResponseError(err) || validationAuthError(err) || !validationReachedUpstream(err))
}

func validationAuthError(err error) bool {
	return core.AsProviderError(err).Kind == core.ErrAuth
}

func validationReachedUpstream(err error) bool {
	return core.AsProviderError(err).StatusCode > 0
}

func strictModelsValidation(provider string) bool {
	switch provider {
	case "openai", "openrouter", "vercel-ai-gateway",
		"deepseek", "groq", "mistral", "perplexity", "together",
		"fireworks", "cerebras", "cohere", "nebius", "siliconflow",
		"hyperbolic", "chutes", "nvidia", "xiaomi-mimo", "xiaomi-tokenplan":
		return true
	default:
		return false
	}
}

// OpenAICompatibleModelSource implements LiveModelSource by fetching the
// upstream GET /models endpoint — the standard discovery API for OpenAI-
// compatible providers. This auto-discovers models at runtime using the
// connected account's credentials.
type OpenAICompatibleModelSource struct {
	provider    string
	defaultBase string
}

// ListModels fetches GET /models from the upstream and returns ModelSpecs.
func (s *OpenAICompatibleModelSource) ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error) {
	url := joinURL(resolvedBaseURL(s.defaultBase, creds), "models")
	req, err := openAIModelsRequest(ctx, url, creds)
	if err != nil {
		return nil, err
	}
	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, modelsStatusError(resp)
	}
	return decodeOpenAIModels(resp.Body)
}

func openAIModelsRequest(ctx context.Context, url string, creds core.Credentials) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token := credentialToken(creds); token != "" {
		req.Header.Set("Authorization", bearer(token))
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func modelsStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	return fmt.Errorf("GET /models returned %d: %s", resp.StatusCode, truncateError(body))
}

func decodeOpenAIModels(body io.Reader) ([]ModelSpec, error) {
	var envelope struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode /models response: %w", err)
	}
	out := make([]ModelSpec, 0, len(envelope.Data))
	for _, entry := range envelope.Data {
		if entry.ID == "" {
			continue
		}
		out = append(out, ModelSpec{
			ID:   entry.ID,
			Name: entry.ID, // best-effort; custom models may carry a better name
			Kind: core.ServiceLLM,
		})
	}
	return out, nil
}

// StreamRaw opens a streaming SSE connection and returns the raw response body
// for zero-copy same-dialect piping. The caller must close body when done.
func (c *OpenAICompatible) StreamRaw(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (io.ReadCloser, http.Header, error) {
	req.Stream = true
	body, err := c.codec.RenderRequestForProvider(req, c.id)
	if err != nil {
		return nil, nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := c.chatCompletionsURL(creds, req.Model)
	resp, err := openStream(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.Header, nil
}

// Stream performs a streaming completion, emitting canonical chunks.
func (c *OpenAICompatible) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	resp, err := c.openChatStream(ctx, req, creds)
	if err != nil {
		return nil, err
	}
	return c.streamResponse(ctx, req.Model, cfg, resp.Body), nil
}

func (c *OpenAICompatible) openChatStream(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*http.Response, error) {
	req.Stream = true
	body, err := c.codec.RenderRequestForProvider(req, c.id)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return openStream(ctx, c.id, req.Model, c.chatCompletionsURL(creds, req.Model), body, c.headers(creds))
}

func (c *OpenAICompatible) streamResponse(ctx context.Context, model string, cfg core.StreamConfig, body io.ReadCloser) <-chan core.StreamChunk {
	out := make(chan core.StreamChunk, 16)
	go func() {
		defer close(out)
		defer body.Close()
		ttft := newTTFTTracker(cfg)
		scanner, sseRelease := sseScanner(body)
		defer sseRelease()
		for scanner.Scan() {
			if !c.streamSSELine(ctx, model, ttft, out, scanner.Text()) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			out <- core.StreamChunk{
				Type: core.ChunkError,
				Err:  &core.ProviderError{Kind: core.ErrTimeout, Provider: c.id, Model: model, Message: err.Error(), Cause: err},
			}
		}
	}()
	return out
}

func (c *OpenAICompatible) streamSSELine(ctx context.Context, model string, ttft *ttftTracker, out chan<- core.StreamChunk, line string) bool {
	if ctx.Err() != nil {
		return false
	}
	payload, ok := parseSSEData(line)
	if !ok {
		return true
	}
	chunks, err := c.codec.ParseStreamLine([]byte(payload), model)
	if err != nil {
		// Skip a single malformed chunk rather than aborting the stream.
		return true
	}
	return sendStreamChunks(ctx, ttft, out, chunks)
}

func sendStreamChunks(ctx context.Context, ttft *ttftTracker, out chan<- core.StreamChunk, chunks []core.StreamChunk) bool {
	for _, ch := range chunks {
		ttft.maybeReport(ch)
		select {
		case out <- ch:
		case <-ctx.Done():
			return false
		}
	}
	return true
}
