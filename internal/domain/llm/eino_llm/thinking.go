package eino_llm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	defaultThinkingEffort    = "medium"
	reasoningContentMarker   = "\"reasoning_content\""
	thinkingTrackerConfigKey = "__thinking_tracker"
)

type thinkingConfig struct {
	Mode          string `json:"mode"`
	BudgetTokens  *int   `json:"budget_tokens,omitempty"`
	Effort        string `json:"effort,omitempty"`
	ClearThinking *bool  `json:"clear_thinking,omitempty"`
}

type openAICompatibleConfig struct {
	Type        string          `json:"type"`
	Provider    string          `json:"provider"`
	ModelName   string          `json:"model_name"`
	APIKey      string          `json:"api_key"`
	BaseURL     string          `json:"base_url"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float32        `json:"temperature,omitempty"`
	TopP        *float32        `json:"top_p,omitempty"`
	Streamable  *bool           `json:"streamable,omitempty"`
	APIVersion  string          `json:"api_version,omitempty"`
	Thinking    *thinkingConfig `json:"thinking,omitempty"`
}

func (t thinkingConfig) enabled() bool {
	return strings.TrimSpace(t.Mode) != "" && t.Mode != "default"
}

func decodeConfigMap(input map[string]interface{}, target interface{}) error {
	if len(input) == 0 {
		return nil
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}

	return json.Unmarshal(payload, target)
}

func decodeOpenAICompatibleConfig(config map[string]interface{}) (openAICompatibleConfig, error) {
	var parsed openAICompatibleConfig
	err := decodeConfigMap(config, &parsed)
	if err != nil {
		return openAICompatibleConfig{}, err
	}

	parsed.Provider = strings.ToLower(strings.TrimSpace(parsed.Provider))
	parsed.Type = strings.ToLower(strings.TrimSpace(parsed.Type))
	parsed.ModelName = strings.TrimSpace(parsed.ModelName)
	parsed.APIKey = strings.TrimSpace(parsed.APIKey)
	parsed.BaseURL = strings.TrimSpace(parsed.BaseURL)
	parsed.APIVersion = strings.TrimSpace(parsed.APIVersion)
	parsed.Thinking = normalizeThinkingConfig(parsed.Thinking)

	return parsed, nil
}

func normalizeThinkingConfig(raw *thinkingConfig) *thinkingConfig {
	if raw == nil {
		return nil
	}

	normalized := &thinkingConfig{
		Mode:          strings.ToLower(strings.TrimSpace(raw.Mode)),
		BudgetTokens:  raw.BudgetTokens,
		Effort:        strings.ToLower(strings.TrimSpace(raw.Effort)),
		ClearThinking: raw.ClearThinking,
	}

	if normalized.Mode == "" && normalized.BudgetTokens == nil && normalized.Effort == "" && normalized.ClearThinking == nil {
		return nil
	}

	return normalized
}

type thinkingRoundTripper struct {
	base     http.RoundTripper
	provider string
	thinking thinkingConfig
	tracker  *reasoningContentTracker
}

type reasoningContentTracker struct {
	returned atomic.Bool
}

func (t *reasoningContentTracker) MarkReturned() {
	if t != nil {
		t.returned.Store(true)
	}
}

func (t *reasoningContentTracker) HasReturned() bool {
	return t != nil && t.returned.Load()
}

func (t *reasoningContentTracker) Reset() {
	if t != nil {
		t.returned.Store(false)
	}
}

type reasoningDetectReadCloser struct {
	io.ReadCloser
	tracker *reasoningContentTracker
	tail    string
}

func (r *reasoningDetectReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 && r.tracker != nil && !r.tracker.HasReturned() {
		chunk := r.tail + string(p[:n])
		if strings.Contains(chunk, reasoningContentMarker) {
			r.tracker.MarkReturned()
		}
		if len(chunk) > len(reasoningContentMarker) {
			r.tail = chunk[len(chunk)-len(reasoningContentMarker):]
		} else {
			r.tail = chunk
		}
	}
	return n, err
}

func (t *thinkingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.Body == nil || !t.thinking.enabled() {
		return t.roundTripAndWrap(req)
	}

	if req.Method != http.MethodPost {
		return t.roundTripAndWrap(req)
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()

	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return t.roundTripAndWrap(cloneRequestWithBody(req, bodyBytes))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return t.roundTripAndWrap(cloneRequestWithBody(req, bodyBytes))
	}

	if !injectThinkingPayload(payload, t.provider, t.thinking) {
		return t.roundTripAndWrap(cloneRequestWithBody(req, bodyBytes))
	}

	newBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return t.roundTripAndWrap(cloneRequestWithBody(req, newBody))
}

func (t *thinkingRoundTripper) roundTripAndWrap(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || t.tracker == nil {
		return resp, err
	}
	resp.Body = &reasoningDetectReadCloser{
		ReadCloser: resp.Body,
		tracker:    t.tracker,
	}
	return resp, nil
}

func cloneRequestWithBody(req *http.Request, body []byte) *http.Request {
	cloned := req.Clone(req.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))
	cloned.Header = req.Header.Clone()
	if len(body) > 0 {
		cloned.Header.Set("Content-Length", strconv.Itoa(len(body)))
	} else {
		cloned.Header.Del("Content-Length")
	}
	return cloned
}

func parseThinkingConfig(config map[string]interface{}) thinkingConfig {
	parsed, err := decodeOpenAICompatibleConfig(config)
	if err != nil || parsed.Thinking == nil {
		return thinkingConfig{}
	}

	return *parsed.Thinking
}

func buildThinkingHTTPClient(config map[string]interface{}, base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}

	thinking := parseThinkingConfig(config)
	if !thinking.enabled() {
		return base
	}

	parsed, err := decodeOpenAICompatibleConfig(config)
	if err != nil {
		return base
	}

	provider := parsed.Provider
	if provider == "" {
		return base
	}

	var tracker *reasoningContentTracker
	if rawTracker, ok := config[thinkingTrackerConfigKey].(*reasoningContentTracker); ok {
		tracker = rawTracker
	}

	cloned := *base
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	cloned.Transport = &thinkingRoundTripper{
		base:     transport,
		provider: provider,
		thinking: thinking,
		tracker:  tracker,
	}
	return &cloned
}

func injectThinkingPayload(payload map[string]interface{}, provider string, thinking thinkingConfig) bool {
	switch provider {
	case "openai", "azure":
		if isOneOf(thinking.Mode, "none", "minimal", "low", "medium", "high", "xhigh") {
			payload["reasoning_effort"] = thinking.Mode
			return true
		}
	case "anthropic":
		if thinking.Mode == "enabled" {
			if thinking.BudgetTokens == nil || *thinking.BudgetTokens <= 0 {
				return false
			}
			payload["thinking"] = map[string]interface{}{
				"type":          "enabled",
				"budget_tokens": *thinking.BudgetTokens,
			}
			return true
		}
		if thinking.Mode == "adaptive" {
			payload["thinking"] = map[string]interface{}{
				"type": "adaptive",
			}
			payload["output_config"] = map[string]interface{}{
				"effort": normalizeThinkingEffort(thinking.Effort),
			}
			return true
		}
	case "doubao":
		if isOneOf(thinking.Mode, "minimal", "low", "medium", "high") {
			payload["reasoning_effort"] = thinking.Mode
			return true
		}
	case "zhipu", "deepseek":
		if isOneOf(thinking.Mode, "enabled", "disabled") {
			thinkingPayload := map[string]interface{}{
				"type": thinking.Mode,
			}
			if provider == "zhipu" && thinking.ClearThinking != nil {
				thinkingPayload["clear_thinking"] = *thinking.ClearThinking
			}
			payload["thinking"] = thinkingPayload
			return true
		}
	case "aliyun", "siliconflow":
		if thinking.Mode == "enabled" {
			payload["enable_thinking"] = true
			if thinking.BudgetTokens != nil && *thinking.BudgetTokens > 0 {
				payload["thinking_budget"] = *thinking.BudgetTokens
			}
			return true
		}
		if thinking.Mode == "disabled" {
			payload["enable_thinking"] = false
			delete(payload, "thinking_budget")
			return true
		}
	}
	return false
}

func isOneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func normalizeThinkingEffort(effort string) string {
	if isOneOf(effort, "low", "medium", "high", "max") {
		return effort
	}
	return defaultThinkingEffort
}
