package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- Claude token resolution ---

func TestResolveClaudeToken_Explicit(t *testing.T) {
	token, err := ResolveClaudeToken("my-explicit-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "my-explicit-token" {
		t.Errorf("got %q, want %q", token, "my-explicit-token")
	}
}

func TestResolveClaudeToken_EnvVar(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "env-token")
	// Ensure credential files don't interfere.
	t.Setenv("HOME", t.TempDir())

	token, err := ResolveClaudeToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "env-token" {
		t.Errorf("got %q, want %q", token, "env-token")
	}
}

func TestResolveClaudeToken_CredentialsFile_DotClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Clear env var so the file path is tried.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	credDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(credDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	creds := claudeCredentials{OAuthToken: "file-token-1"}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(filepath.Join(credDir, "credentials.json"), data, 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	token, err := ResolveClaudeToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "file-token-1" {
		t.Errorf("got %q, want %q", token, "file-token-1")
	}
}

func TestResolveClaudeToken_CredentialsFile_DotConfigClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	// Only write the second candidate path.
	credDir := filepath.Join(home, ".config", "claude")
	if err := os.MkdirAll(credDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	creds := claudeCredentials{OAuthToken: "file-token-2"}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(filepath.Join(credDir, "credentials.json"), data, 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	token, err := ResolveClaudeToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "file-token-2" {
		t.Errorf("got %q, want %q", token, "file-token-2")
	}
}

func TestResolveClaudeToken_FirstFileTakesPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	// Write both credential files; the first (~/.claude) should win.
	for _, sub := range []string{".claude", filepath.Join(".config", "claude")} {
		dir := filepath.Join(home, sub)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		creds := claudeCredentials{OAuthToken: sub + "-token"}
		data, _ := json.Marshal(creds)
		if err := os.WriteFile(filepath.Join(dir, "credentials.json"), data, 0600); err != nil {
			t.Fatalf("write credentials: %v", err)
		}
	}

	token, err := ResolveClaudeToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != ".claude-token" {
		t.Errorf("got %q, want %q", token, ".claude-token")
	}
}

func TestResolveClaudeToken_NoToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	token, err := ResolveClaudeToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestResolveClaudeToken_ExplicitBeatsEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "env-token")

	token, err := ResolveClaudeToken("explicit-wins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "explicit-wins" {
		t.Errorf("got %q, want explicit-wins", token)
	}
}

// --- GitHub token resolution ---

func TestResolveGitHubToken_Explicit(t *testing.T) {
	token, err := ResolveGitHubToken("gh-explicit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "gh-explicit" {
		t.Errorf("got %q, want %q", token, "gh-explicit")
	}
}

func TestResolveGitHubToken_GITHUB_TOKEN(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "github-env-token")
	t.Setenv("GH_TOKEN", "")

	token, err := ResolveGitHubToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "github-env-token" {
		t.Errorf("got %q, want %q", token, "github-env-token")
	}
}

func TestResolveGitHubToken_GH_TOKEN(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh-env-token")

	token, err := ResolveGitHubToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "gh-env-token" {
		t.Errorf("got %q, want %q", token, "gh-env-token")
	}
}

func TestResolveGitHubToken_GITHUB_TOKEN_BeatsGH_TOKEN(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "github-wins")
	t.Setenv("GH_TOKEN", "gh-loses")

	token, err := ResolveGitHubToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "github-wins" {
		t.Errorf("got %q, want github-wins", token)
	}
}

func TestResolveGitHubToken_ExplicitBeatsEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")

	token, err := ResolveGitHubToken("explicit-wins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "explicit-wins" {
		t.Errorf("got %q, want explicit-wins", token)
	}
}

func TestResolveGitHubToken_Anonymous(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	// ghAuthToken() will fail because gh is either not installed or not
	// authenticated in CI — either way the result should be empty.

	token, err := ResolveGitHubToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// We cannot assert token == "" because gh might be installed and
	// authenticated in the test environment. We just check no error.
	_ = token
}
