package tools

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed slack_notify.md
var slackNotifyDescription string

// SlackNotify posts a notification to Slack via an incoming webhook. It is the
// single gated write action in v1: class action_execute, so it can only run in
// the Execute phase with an approval on record. If RC.Notifier is set (tests,
// harness, or a configured sink) it is used instead of the live webhook.
type SlackNotify struct {
	RC         *RunContext
	WebhookURL string
	HTTP       *http.Client
}

func (t *SlackNotify) Name() string        { return "slack_notify" }
func (t *SlackNotify) Description() string { return slackNotifyDescription }
func (t *SlackNotify) ReadOnly() bool      { return false }

func (t *SlackNotify) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"message": map[string]any{"type": "string", "description": "The notification text to post."},
		},
		Required: []string{"message"},
	}
}

func (t *SlackNotify) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct{ Message string }
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Message == "" {
		return tool.Errorf("message is required"), nil
	}

	if t.RC != nil && t.RC.Notifier != nil {
		ptr, err := t.RC.Notifier.Notify(in.Message)
		if err != nil {
			return tool.Errorf("notification failed: %v", err), nil
		}
		return tool.Text("notification sent (" + ptr + ")"), nil
	}

	if t.WebhookURL == "" {
		return tool.Errorf("no Slack webhook configured (set SLACK_WEBHOOK_URL)"), nil
	}
	body, _ := json.Marshal(map[string]string{"text": in.Message})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return tool.Errorf("build request: %v", err), nil
	}
	req.Header.Set("Content-Type", "application/json")
	cli := t.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := cli.Do(req)
	if err != nil {
		return tool.Errorf("post failed: %v", err), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return tool.Errorf("slack returned status %d", resp.StatusCode), nil
	}
	return tool.Text(fmt.Sprintf("notification sent (slack:%d)", resp.StatusCode)), nil
}
