package shared

import "testing"

func TestReplacePlaceholders_AllFields(t *testing.T) {
	cfg := McpServerConfig{
		Command: "${command}",
		Args:    []string{"${workspaceFolder}/server", "--root=${workspaceFolder}"},
		Env:     map[string]string{"ROOT": "${workspaceFolder}"},
		Type:    "http",
		Url:     "http://${host}/mcp",
		Headers: map[string]string{"Authorization": "Bearer ${token}"},
	}
	replacements := map[string]string{
		"${command}":         "npx",
		"${workspaceFolder}": "/repo",
		"${host}":            "localhost:8080",
		"${token}":           "secret",
	}

	got := cfg.ReplacePlaceholders(replacements)

	if got.Command != "npx" {
		t.Errorf("Command = %q", got.Command)
	}
	if len(got.Args) != 2 || got.Args[0] != "/repo/server" || got.Args[1] != "--root=/repo" {
		t.Errorf("Args = %#v", got.Args)
	}
	if got.Env["ROOT"] != "/repo" {
		t.Errorf("Env = %#v", got.Env)
	}
	if got.Url != "http://localhost:8080/mcp" {
		t.Errorf("Url = %q", got.Url)
	}
	if got.Headers["Authorization"] != "Bearer secret" {
		t.Errorf("Headers = %#v", got.Headers)
	}
}

func TestReplacePlaceholders_DoesNotMutateOriginal(t *testing.T) {
	cfg := McpServerConfig{
		Env:     map[string]string{"ROOT": "${workspaceFolder}"},
		Headers: map[string]string{"X-Root": "${workspaceFolder}"},
	}

	_ = cfg.ReplacePlaceholders(map[string]string{"${workspaceFolder}": "/repo"})

	if cfg.Env["ROOT"] != "${workspaceFolder}" {
		t.Errorf("original Env was mutated: %#v", cfg.Env)
	}
	if cfg.Headers["X-Root"] != "${workspaceFolder}" {
		t.Errorf("original Headers were mutated: %#v", cfg.Headers)
	}
}
