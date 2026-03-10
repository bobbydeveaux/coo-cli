package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveClaudeToken resolves the Claude Code OAuth token using the following
// priority order:
//  1. explicit value (from --token flag)
//  2. CLAUDE_CODE_OAUTH_TOKEN environment variable
//  3. ~/.claude/credentials.json
//  4. ~/.config/claude/credentials.json
//
// Returns an empty string (no error) if no token is found anywhere.
func ResolveClaudeToken(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	if v := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); v != "" {
		return v, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	candidates := []string{
		filepath.Join(home, ".claude", "credentials.json"),
		filepath.Join(home, ".config", "claude", "credentials.json"),
	}

	for _, p := range candidates {
		token, err := readClaudeCredentials(p)
		if err == nil && token != "" {
			return token, nil
		}
	}

	return "", nil
}

// claudeCredentials is the minimal shape of Claude Code's credentials.json.
type claudeCredentials struct {
	OAuthToken string `json:"claudeAiOauthToken"`
}

// readClaudeCredentials reads the OAuth token from a credentials.json file.
// Returns an empty string (no error) if the file does not exist.
func readClaudeCredentials(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	var creds claudeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return creds.OAuthToken, nil
}

// ResolveGitHubToken resolves the GitHub token using the following priority order:
//  1. explicit value (from --github-token flag)
//  2. GITHUB_TOKEN environment variable
//  3. GH_TOKEN environment variable
//  4. output of `gh auth token` (if gh CLI is installed and authenticated)
//  5. empty string (anonymous — public repos only)
func ResolveGitHubToken(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	if v := os.Getenv("GITHUB_TOKEN"); v != "" {
		return v, nil
	}

	if v := os.Getenv("GH_TOKEN"); v != "" {
		return v, nil
	}

	if token := ghAuthToken(); token != "" {
		return token, nil
	}

	return "", nil
}

// ghAuthToken runs `gh auth token` and returns its trimmed output.
// Returns an empty string if gh is not installed, not in PATH, or not authenticated.
func ghAuthToken() string {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
