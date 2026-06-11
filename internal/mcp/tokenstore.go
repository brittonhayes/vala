package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
)

// tokenRecord is everything needed to reuse and refresh an MCP server's OAuth
// authorization across sessions: the dynamically registered client, the
// authorization-server endpoints, and the token itself.
type tokenRecord struct {
	ClientID      string        `json:"client_id"`
	ClientSecret  string        `json:"client_secret,omitempty"`
	AuthEndpoint  string        `json:"auth_endpoint"`
	TokenEndpoint string        `json:"token_endpoint"`
	Redirect      string        `json:"redirect,omitempty"`
	Scopes        []string      `json:"scopes,omitempty"`
	Token         *oauth2.Token `json:"token,omitempty"`
}

// config builds the oauth2 config used to refresh and reuse the token.
func (r tokenRecord) config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     r.ClientID,
		ClientSecret: r.ClientSecret,
		RedirectURL:  r.Redirect,
		Scopes:       r.Scopes,
		Endpoint:     oauth2.Endpoint{AuthURL: r.AuthEndpoint, TokenURL: r.TokenEndpoint},
	}
}

// tokenStore persists MCP OAuth records to a 0600 file, keyed by server name. It
// is the out-of-band token cache the config file deliberately never holds.
type tokenStore struct {
	path string
	mu   sync.Mutex
}

// defaultTokenStore returns the store backed by ~/.config/vala/mcp-auth.json.
func defaultTokenStore() *tokenStore {
	dir, err := os.UserConfigDir()
	if err != nil {
		return &tokenStore{}
	}
	return &tokenStore{path: filepath.Join(dir, "vala", "mcp-auth.json")}
}

type tokenFile struct {
	Servers map[string]tokenRecord `json:"servers"`
}

func (s *tokenStore) load() tokenFile {
	tf := tokenFile{Servers: map[string]tokenRecord{}}
	if s.path == "" {
		return tf
	}
	data, err := os.ReadFile(s.path)
	if err != nil || len(data) == 0 {
		return tf
	}
	_ = json.Unmarshal(data, &tf)
	if tf.Servers == nil {
		tf.Servers = map[string]tokenRecord{}
	}
	return tf
}

func (s *tokenStore) get(server string) (tokenRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.load().Servers[server]
	return rec, ok
}

func (s *tokenStore) set(server string, rec tokenRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	tf := s.load()
	tf.Servers[server] = rec
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
