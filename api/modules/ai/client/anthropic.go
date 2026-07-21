package aiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"maintenancehub/modules/ai"
)

// Client wraps the Anthropic API for classification calls. Ported from
// ra-avm's ai/client with the document-parsing surface (insurance, utility
// bills, vendor bills, registrations) removed — none of those product areas
// exist here.
type Client struct {
	client anthropic.Client
	model  anthropic.Model

	// When set, Ask routes to OpenRouter's OpenAI-compatible endpoint instead of
	// the Anthropic SDK. See openrouter.go.
	openrouter *openRouterConfig
}

func New(apiKey string) *Client {
	model := os.Getenv("AI_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &Client{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  anthropic.Model(model),
	}
}

// Classify sends the system+user prompt and parses the JSON response.
func (c *Client) Classify(ctx context.Context, systemPrompt, userPrompt string) (*ai.AIClassificationResult, string, error) {
	rawText, err := c.Ask(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, "", err
	}

	jsonStr := ExtractJSON(rawText)
	var result ai.AIClassificationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, rawText, fmt.Errorf("failed to parse AI response as JSON: %w (raw: %s)", err, rawText)
	}
	return &result, rawText, nil
}

// ClassifySMS classifies an SMS conversation — same call shape as Classify.
func (c *Client) ClassifySMS(ctx context.Context, systemPrompt, userPrompt string) (*ai.AIClassificationResult, string, error) {
	return c.Classify(ctx, systemPrompt, userPrompt)
}

// Ask sends a system+user prompt and returns the raw text response — used
// for free-form tasks like vendor-reply quote extraction.
func (c *Client) Ask(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.openrouter != nil {
		return c.askOpenRouter(ctx, systemPrompt, userPrompt)
	}
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 2048,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(userPrompt),
			),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic API error: %w", err)
	}

	var rawText string
	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			rawText = v.Text
		}
		if rawText != "" {
			break
		}
	}
	if rawText == "" {
		return "", fmt.Errorf("empty response from Claude")
	}
	return rawText, nil
}

// ExtractJSON strips markdown code fences from a model response, falling
// back to the outermost {...} span.
func ExtractJSON(s string) string {
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
	}
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		if start := strings.Index(s, "{"); start >= 0 {
			if end := strings.LastIndex(s, "}"); end > start {
				s = s[start : end+1]
			}
		}
	}
	return s
}
