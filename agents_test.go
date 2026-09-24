package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAddMCPServer(t *testing.T) {
	readServers := func(t *testing.T, path string) map[string]map[string]any {
		t.Helper()
		var config struct {
			MCPServers map[string]map[string]any `json:"mcpServers"`
		}
		data, _ := os.ReadFile(path)
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatalf("the file is not JSON anymore: %v\n%s", err, data)
		}
		return config.MCPServers
	}

	t.Run("creates the file when there is none", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".cursor", "mcp.json")

		err := addMCPServer(path)

		servers := readServers(t, path)
		if err != nil || servers["lognorth"]["args"].([]any)[0] != "mcp" {
			t.Errorf("err = %v, servers = %v, want lognorth running north mcp", err, servers)
		}
	})

	t.Run("keeps the other servers and settings", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "mcp.json")
		os.WriteFile(path, []byte(`{"theme":"dark","mcpServers":{"notes":{"command":"node","env":{"VAULT":"/x"}}}}`), 0o644)

		addMCPServer(path)
		addMCPServer(path) // twice changes nothing more

		servers := readServers(t, path)
		data, _ := os.ReadFile(path)
		if len(servers) != 2 || servers["notes"]["command"] != "node" || !json.Valid(data) {
			t.Errorf("servers = %v, want notes kept next to lognorth", servers)
		}
		var config map[string]any
		json.Unmarshal(data, &config)
		if config["theme"] != "dark" {
			t.Errorf("config = %v, want the theme setting kept", config)
		}
	})

	t.Run("leaves a file it cannot read alone", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "mcp.json")
		os.WriteFile(path, []byte(`{ "mcpServers": { // a comment`), 0o644)

		err := addMCPServer(path)

		data, _ := os.ReadFile(path)
		if err == nil || string(data) != `{ "mcpServers": { // a comment` {
			t.Errorf("err = %v, file = %s, want an error and the file unchanged", err, data)
		}
	})
}
