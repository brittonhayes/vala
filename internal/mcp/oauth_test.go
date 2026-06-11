package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestTokenStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &tokenStore{path: filepath.Join(dir, "mcp-auth.json")}

	if _, ok := s.get("wiz"); ok {
		t.Fatal("empty store should report no record")
	}
	rec := tokenRecord{
		ClientID:      "abc",
		AuthEndpoint:  "https://as/authorize",
		TokenEndpoint: "https://as/token",
		Token:         &oauth2.Token{AccessToken: "tok", RefreshToken: "ref"},
	}
	if err := s.set("wiz", rec); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok := s.get("wiz")
	if !ok || got.ClientID != "abc" || got.Token.AccessToken != "tok" {
		t.Fatalf("round-trip mismatch: %+v (ok=%v)", got, ok)
	}

	// The token file must be operator-only.
	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("token file mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestOAuthHandlerEndToEnd drives the full handler against a fake authorization
// server: discovery → dynamic client registration → loopback authorization →
// code exchange, then confirms the token is cached and reused.
func TestOAuthHandlerEndToEnd(t *testing.T) {
	var as *httptest.Server
	as = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			writeJSON(w, map[string]any{
				"resource":              as.URL,
				"authorization_servers": []string{as.URL},
				"scopes_supported":      []string{"read"},
			})
		case "/.well-known/oauth-authorization-server":
			writeJSON(w, map[string]any{
				"issuer":                           as.URL,
				"authorization_endpoint":           as.URL + "/authorize",
				"token_endpoint":                   as.URL + "/token",
				"registration_endpoint":            as.URL + "/register",
				"response_types_supported":         []string{"code"},
				"code_challenge_methods_supported": []string{"S256"},
			})
		case "/register":
			writeJSON(w, map[string]any{"client_id": "dyn-client", "redirect_uris": []string{}})
		case "/token":
			writeJSON(w, map[string]any{"access_token": "granted-token", "token_type": "Bearer", "refresh_token": "r1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer as.Close()

	dir := t.TempDir()
	h := &oauthHandler{
		server: "wiz",
		store:  &tokenStore{path: filepath.Join(dir, "mcp-auth.json")},
		hc:     as.Client(),
	}
	// Instead of a real browser, immediately hit the loopback redirect with a code.
	h.open = func(authURL string) {
		u, err := url.Parse(authURL)
		if err != nil {
			t.Errorf("bad auth url: %v", err)
			return
		}
		redirect := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		go http.Get(redirect + "?code=the-code&state=" + state)
	}

	// A handler with no token yet returns a nil source (→ triggers Authorize).
	ts, err := h.TokenSource(context.Background())
	if err != nil {
		t.Fatalf("TokenSource: %v", err)
	}
	if ts != nil {
		t.Fatal("expected nil token source before authorization")
	}

	// Simulate the transport's 401 → Authorize path.
	req, _ := http.NewRequest("POST", as.URL+"/mcp", nil)
	resp := &http.Response{StatusCode: 401, Header: http.Header{}, Body: http.NoBody, Request: req}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.Authorize(ctx, req, resp); err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	// The token must now be cached and served.
	ts, err = h.TokenSource(ctx)
	if err != nil || ts == nil {
		t.Fatalf("expected a token source after Authorize (err=%v)", err)
	}
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "granted-token" {
		t.Errorf("access token = %q, want granted-token", tok.AccessToken)
	}
	if rec, ok := h.store.get("wiz"); !ok || rec.ClientID != "dyn-client" {
		t.Errorf("registered client not persisted: %+v", rec)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
