package setup

import (
	"context"
	"os"
	"os/exec"
	"runtime"

	"github.com/brittonhayes/vala/internal/auth"
	"github.com/brittonhayes/vala/internal/auth/oauth"
	"github.com/brittonhayes/vala/internal/config"
	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

// oauthExchangedMsg carries the result of trading the pasted code for tokens.
type oauthExchangedMsg struct {
	cred auth.Credential
	err  error
}

// evidenceValidatedMsg carries the result of dialing a just-saved evidence
// source and listing its tools.
type evidenceValidatedMsg struct {
	status mcp.EvidenceStatus
}

// authorizeOAuth mints the consent URL and PKCE verifier for the subscription
// login. It is a thin wrapper so the model does not import the oauth package.
func authorizeOAuth() (oauth.Authorization, error) {
	return oauth.AnthropicAuthorize()
}

// exchangeOAuthCmd trades the pasted code for an OAuth credential off the UI
// goroutine.
func exchangeOAuthCmd(ctx context.Context, code, verifier string) tea.Cmd {
	return func() tea.Msg {
		tok, err := oauth.AnthropicExchange(ctx, code, verifier)
		if err != nil {
			return oauthExchangedMsg{err: err}
		}
		return oauthExchangedMsg{cred: auth.Credential{
			Type:    "oauth",
			Access:  tok.Access,
			Refresh: tok.Refresh,
			Expiry:  tok.Expiry.UnixMilli(),
		}}
	}
}

// validateEvidenceCmd dials a saved evidence source and discovers its tools, so
// the operator gets a live ✓/✗ instead of finding out at the first hunt. The
// secrets are resolved from the environment here, never persisted.
func validateEvidenceCmd(ctx context.Context, srv config.MCPServer) tea.Cmd {
	return func() tea.Msg {
		_, status := tools.ConnectEvidence(ctx, resolveServer(srv))
		return evidenceValidatedMsg{status: status}
	}
}

// resolveServer fills a persisted server's secrets from the environment: the
// bearer token for an HTTP server and any passthrough variables for a stdio one.
func resolveServer(srv config.MCPServer) mcp.ServerConfig {
	c := mcp.ServerConfig{
		Name:      srv.Name,
		Transport: srv.Transport,
		URL:       srv.URL,
		OAuth:     srv.OAuth,
		Command:   srv.Command,
		Args:      srv.Args,
	}
	if srv.APIKeyEnv != "" {
		c.APIKey = os.Getenv(srv.APIKeyEnv)
	}
	for _, name := range srv.EnvPassthrough {
		if v, ok := os.LookupEnv(name); ok {
			if c.Env == nil {
				c.Env = make(map[string]string)
			}
			c.Env[name] = v
		}
	}
	return c
}

// openBrowser best-effort opens a URL in the operator's default browser; the URL
// is always shown on screen as a fallback.
func openBrowser(url string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		name = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(name, args...).Start()
}
