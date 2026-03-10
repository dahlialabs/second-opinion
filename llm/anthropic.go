package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	AnthropicURL        = "https://api.anthropic.com/v1/messages"
	AnthropicAPIVersion = "2023-06-01"
	anthropicProvider    = "anthropic"
	anthropicSystemPrompt = `You are a code reviewer. Find real problems — bugs, logic errors, runtime failures, security vulnerabilities, and poor engineering decisions.

Rules:
- Facts only. Every finding must be backed by specific evidence in the code. Explain exactly what breaks, with what input, and what the observable consequence is. No speculative concerns.
- If you reason through something and conclude it's fine, drop it. Never include findings you've talked yourself out of.
- Never flag: style, naming conventions, missing comments/docs, TODOs, import ordering, type annotations, cosmetic issues.
- Be proportional. Small changes get brief reviews. If nothing is wrong, say so in one sentence and stop.
- Do not re-summarize the input — jump straight to findings.

Engineering principles to enforce:
- Flag code that does destructive operations (delete, overwrite, replace) without first saving/committing the previous state.
- Flag long-running operations with no logging, no progress output, or no way to observe what's happening. Scripts should log to files and flush output.
- Flag string comparison / regex used for classification or pattern detection where an LLM or semantic approach would be more robust.
- Flag hardcoded values that should be configurable, and decision logic scattered across files that should be centralized.
- Flag unnecessary abstractions, premature generalization, and scope creep. Three clear lines is better than one clever abstraction.
- Flag one-off symptom patches that leave the root cause intact.`
)

// AnthropicProvider implements the Provider interface for Anthropic (Claude)
type AnthropicProvider struct {
	apiKey      string
	model       string
	temperature float64
	maxTokens   int
	retryConfig RetryConfig
	httpClient  *http.Client
}

type anthropicRequest struct {
	Model       string         `json:"model"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature float64        `json:"temperature,omitempty"`
	System      string         `json:"system"`
	Messages    []anthropicMsg `json:"messages"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(config Config) (*AnthropicProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Anthropic API key is required")
	}

	model := config.Model
	if model == "" {
		model = "claude-opus-4-20250514"
	}

	temperature := config.Temperature

	maxTokens := config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	return &AnthropicProvider{
		apiKey:      config.APIKey,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		retryConfig: DefaultRetryConfig(),
		httpClient:  SharedHTTPClient,
	}, nil
}

// Analyze sends a prompt to Anthropic and returns the response
func (p *AnthropicProvider) Analyze(ctx context.Context, prompt string) (string, error) {
	reqBody := anthropicRequest{
		Model:       p.model,
		MaxTokens:   p.maxTokens,
		Temperature: p.temperature,
		System:      anthropicSystemPrompt,
		Messages: []anthropicMsg{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", AnthropicURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", AnthropicAPIVersion)

	resp, err := RetryableHTTPRequest(ctx, p.httpClient, req, p.retryConfig)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Redact API key from error message if present
		if p.apiKey != "" && len(p.apiKey) > 8 {
			return "", fmt.Errorf("Anthropic API error (status %d): [response body redacted for security]", resp.StatusCode)
		}
		return "", fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result anthropicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from Anthropic")
	}

	// Extract text from content blocks
	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in Anthropic response")
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return anthropicProvider
}
