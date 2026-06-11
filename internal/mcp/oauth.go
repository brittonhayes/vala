package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// oauthFlowTimeout bounds the whole browser authorization round-trip so a server
// that is reached but never authorized cannot hang the session forever.
const oauthFlowTimeout = 3 * time.Minute

// oauthHandler authorizes an HTTP MCP server with the MCP OAuth flow: it
// discovers the authorization server from the 401 response, registers a client
// dynamically, runs a loopback browser sign-in, and persists the resulting token
// so later launches reuse it (refreshing silently) instead of re-prompting. It
// implements auth.OAuthHandler, which StreamableClientTransport calls to set the
// Authorization header and to recover from a 401/403.
type oauthHandler struct {
	server string
	store  *tokenStore
	open   func(string)
	hc     *http.Client

	mu sync.Mutex
	ts oauth2.TokenSource
}

func newOAuthHandler(server string, store *tokenStore) *oauthHandler {
	return &oauthHandler{server: server, store: store, open: openBrowser, hc: http.DefaultClient}
}

// TokenSource returns a cached, auto-refreshing token source when a token is
// stored, or nil to let the transport proceed unauthenticated (which yields the
// 401 that triggers Authorize).
func (h *oauthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ts != nil {
		return h.ts, nil
	}
	rec, ok := h.store.get(h.server)
	if !ok || rec.Token == nil {
		return nil, nil
	}
	h.ts = h.persisting(rec.config().TokenSource(ctx, rec.Token))
	return h.ts, nil
}

// Authorize runs the OAuth flow after a 401/403 and installs the resulting token
// source. The transport retries the request once Authorize returns nil.
func (h *oauthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	defer resp.Body.Close()

	ctx, cancel := context.WithTimeout(ctx, oauthFlowTimeout)
	defer cancel()

	rec, err := h.runFlow(ctx, req, resp)
	if err != nil {
		return fmt.Errorf("oauth flow for %q: %w", h.server, err)
	}
	if err := h.store.set(h.server, rec); err != nil {
		return fmt.Errorf("persist token for %q: %w", h.server, err)
	}
	h.mu.Lock()
	h.ts = h.persisting(rec.config().TokenSource(ctx, rec.Token))
	h.mu.Unlock()
	return nil
}

// runFlow performs discovery, dynamic client registration, the loopback browser
// authorization, and the code exchange, returning a persistable token record.
func (h *oauthHandler) runFlow(ctx context.Context, req *http.Request, resp *http.Response) (tokenRecord, error) {
	resource := (&url.URL{Scheme: req.URL.Scheme, Host: req.URL.Host}).String()

	// 1. Locate the protected-resource metadata, then the authorization server.
	prmURL := resourceMetadataURL(resp, req)
	prm, err := oauthex.GetProtectedResourceMetadata(ctx, prmURL, resource, h.hc)
	if err != nil {
		return tokenRecord{}, fmt.Errorf("discover protected resource: %w", err)
	}
	if len(prm.AuthorizationServers) == 0 {
		return tokenRecord{}, fmt.Errorf("server advertises no authorization servers")
	}
	issuer := prm.AuthorizationServers[0]
	asm, err := oauthex.GetAuthServerMeta(ctx, authServerMetadataURL(issuer), issuer, h.hc)
	if err != nil {
		return tokenRecord{}, fmt.Errorf("discover authorization server: %w", err)
	}

	// 2. Bind a loopback listener; its address is the OAuth redirect URI.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return tokenRecord{}, fmt.Errorf("open loopback listener: %w", err)
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)

	scopes := prm.ScopesSupported
	if len(scopes) == 0 {
		scopes = asm.ScopesSupported
	}

	// 3. Register a client dynamically (reusing one we registered before).
	rec, _ := h.store.get(h.server)
	rec.AuthEndpoint = asm.AuthorizationEndpoint
	rec.TokenEndpoint = asm.TokenEndpoint
	rec.Scopes = scopes
	rec.Redirect = redirect
	if rec.ClientID == "" {
		if asm.RegistrationEndpoint == "" {
			return tokenRecord{}, fmt.Errorf("server requires preregistration (no dynamic registration endpoint)")
		}
		reg, err := oauthex.RegisterClient(ctx, asm.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{
			RedirectURIs:            []string{redirect},
			TokenEndpointAuthMethod: "none",
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			ClientName:              "vala",
			Scope:                   strings.Join(scopes, " "),
		}, h.hc)
		if err != nil {
			return tokenRecord{}, fmt.Errorf("register client: %w", err)
		}
		rec.ClientID = reg.ClientID
		rec.ClientSecret = reg.ClientSecret
	}

	// 4. Run the browser authorization with PKCE and capture the code.
	cfg := rec.config()
	cfg.RedirectURL = redirect
	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		return tokenRecord{}, err
	}
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier))
	h.open(authURL)

	code, err := waitForCode(ctx, ln, state)
	if err != nil {
		return tokenRecord{}, err
	}

	// 5. Exchange the code for a token (over our SSRF-safe client).
	ctx = context.WithValue(ctx, oauth2.HTTPClient, h.hc)
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return tokenRecord{}, fmt.Errorf("exchange code: %w", err)
	}
	rec.Token = tok
	return rec, nil
}

// persisting wraps a token source so a refreshed token is written back to the
// store, keeping later launches authenticated without another sign-in.
func (h *oauthHandler) persisting(src oauth2.TokenSource) oauth2.TokenSource {
	return &persistingSource{handler: h, src: oauth2.ReuseTokenSource(nil, src)}
}

type persistingSource struct {
	handler *oauthHandler
	src     oauth2.TokenSource
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if rec, ok := p.handler.store.get(p.handler.server); ok {
		if rec.Token == nil || rec.Token.AccessToken != tok.AccessToken {
			rec.Token = tok
			_ = p.handler.store.set(p.handler.server, rec)
		}
	}
	return tok, nil
}

// resourceMetadataURL resolves where to fetch protected-resource metadata: the
// resource_metadata parameter from the 401's WWW-Authenticate challenge if
// present, else the well-known path on the resource host (RFC 9728).
func resourceMetadataURL(resp *http.Response, req *http.Request) string {
	if challenges, err := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate")); err == nil {
		for _, c := range challenges {
			if rm := c.Params["resource_metadata"]; rm != "" {
				return rm
			}
		}
	}
	return (&url.URL{Scheme: req.URL.Scheme, Host: req.URL.Host, Path: "/.well-known/oauth-protected-resource"}).String()
}

// waitForCode serves the single loopback redirect, validates the state, and
// returns the authorization code.
func waitForCode(ctx context.Context, ln net.Listener, state string) (string, error) {
	type result struct {
		code string
		err  error
	}
	ch := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			fmt.Fprintf(w, "Authorization failed: %s. You can close this tab.", e)
			ch <- result{err: fmt.Errorf("authorization denied: %s", e)}
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			ch <- result{err: fmt.Errorf("state mismatch on callback")}
			return
		}
		fmt.Fprint(w, "vala is connected. You can close this tab and return to the terminal.")
		ch <- result{code: q.Get("code")}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		return res.code, res.err
	}
}

// authServerMetadataURL derives the RFC 8414 well-known metadata URL for an
// issuer, inserting the well-known segment after the host and before any path.
func authServerMetadataURL(issuer string) string {
	u, err := url.Parse(issuer)
	if err != nil {
		return issuer
	}
	wellKnown := "/.well-known/oauth-authorization-server"
	if p := strings.Trim(u.Path, "/"); p != "" {
		u.Path = wellKnown + "/" + p
	} else {
		u.Path = wellKnown
	}
	return u.String()
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// openBrowser best-effort opens a URL in the operator's default browser.
func openBrowser(target string) {
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
	args = append(args, target)
	_ = exec.Command(name, args...).Start()
}

// ensure the interface is satisfied at compile time.
var _ auth.OAuthHandler = (*oauthHandler)(nil)
