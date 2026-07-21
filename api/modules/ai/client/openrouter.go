package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

type openRouterConfig struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenRouter returns a Client that calls OpenRouter's OpenAI-compatible
// chat-completions endpoint instead of the Anthropic API. model is an
// OpenRouter model slug (e.g. "google/gemini-2.5-flash-lite" for a cheap model,
// or "anthropic/claude-haiku-4.5" to route to Claude). The classification and
// quote-parsing prompts are unchanged — this only swaps how the prompt is sent.
func NewOpenRouter(apiKey, model string) *Client {
	return &Client{
		openrouter: &openRouterConfig{
			apiKey:     apiKey,
			model:      model,
			httpClient: &http.Client{Timeout: 60 * time.Second},
		},
	}
}

func (c *Client) askOpenRouter(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := map[string]any{
		"model": c.openrouter.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"max_tokens": 2048,
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("openrouter: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.openrouter.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "Maintenance Hub") // optional attribution; harmless

	resp, err := c.openrouter.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openrouter: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter: status %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("openrouter: parse response: %w (raw: %s)", err, string(body))
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("openrouter: empty response (raw: %s)", string(body))
	}
	return parsed.Choices[0].Message.Content, nil
}
