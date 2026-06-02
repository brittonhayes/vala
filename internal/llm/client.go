// Package llm is a thin wrapper over the Anthropic Go SDK so the rest of
// vala depends on a small, stable surface rather than the full client.
package llm

import (
	"context"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/brittonhayes/vala/internal/config"
)

// Client talks to the Anthropic Messages API with a fixed model and token cap.
type Client struct {
	api       anthropic.Client
	model     anthropic.Model
	maxTokens int64
}

// New builds a Client from configuration. It returns an error if no API key is
// available, since every call would otherwise fail at request time.
func New(cfg config.Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("no Anthropic API key: set ANTHROPIC_API_KEY")
	}
	return &Client{
		api:       anthropic.NewClient(option.WithAPIKey(cfg.APIKey)),
		model:     anthropic.Model(cfg.Model),
		maxTokens: cfg.MaxTokens,
	}, nil
}

// Model returns the configured model ID.
func (c *Client) Model() string { return string(c.model) }

// Complete sends one request: a system prompt, the conversation so far, and the
// available tools. It returns the assistant's reply message.
func (c *Client) Complete(
	ctx context.Context,
	system string,
	messages []anthropic.MessageParam,
	tools []anthropic.ToolUnionParam,
) (*anthropic.Message, error) {
	return c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  messages,
		Tools:     tools,
	})
}
