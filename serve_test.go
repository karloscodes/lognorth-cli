package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// relayServer answers raw JSON-RPC the way LogNorth's /mcp does: it echoes
// the request id, and answers a notification (no id) with 202 and no body.
func relayServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		// Pretty-printed on purpose: the relay must send one line per message.
		body, _ := json.MarshalIndent(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"method": req.Method}}, "", "  ")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// connected saves a connection the way north connect does, in a fresh config dir.
func connected(t *testing.T, url, key string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LOGNORTH_URL", "")
	t.Setenv("LOGNORTH_AGENT_KEY", "")
	if url != "" {
		if _, err := saveRemote(remote{URL: url, Key: key}); err != nil {
			t.Fatalf("save: %v", err)
		}
	}
}

func relay(t *testing.T, input string) []string {
	t.Helper()
	var out bytes.Buffer
	if err := serveMCP(strings.NewReader(input), &out); err != nil {
		t.Fatalf("serveMCP: %v", err)
	}
	return strings.FieldsFunc(out.String(), func(r rune) bool { return r == '\n' })
}

func TestMCPRelay(t *testing.T) {
	t.Run("forwards each request and keeps its id", func(t *testing.T) {
		connected(t, relayServer(t), testKey)

		lines := relay(t, `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":"abc","method":"tools/list","params":{}}
`)

		if len(lines) != 2 || !strings.Contains(lines[0], `"id":7`) || !strings.Contains(lines[1], `"id":"abc"`) {
			t.Errorf("replies = %q, want one line per request with its id", lines)
		}
	})

	t.Run("a new north connect takes effect without a restart", func(t *testing.T) {
		connected(t, relayServer(t), "lgn-agent-old")
		url := relayServer(t)
		in, feed := io.Pipe()
		out, replies := io.Pipe()
		go serveMCP(in, replies)
		read := bufio.NewReader(out)

		io.WriteString(feed, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n")
		first, _ := read.ReadString('\n')
		saveRemote(remote{URL: url, Key: testKey})
		io.WriteString(feed, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`+"\n")
		second, _ := read.ReadString('\n')
		feed.Close()

		if !strings.Contains(first, "rejected") || !strings.Contains(second, `"result"`) {
			lines := []string{first, second}
			t.Errorf("replies = %q, want the old key rejected, then the new one answered", lines)
		}
	})

	t.Run("a notification gets no reply", func(t *testing.T) {
		connected(t, relayServer(t), testKey)

		lines := relay(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")

		if len(lines) != 0 {
			t.Errorf("replies = %q, want none", lines)
		}
	})

	t.Run("a rejected key answers with what to run", func(t *testing.T) {
		connected(t, relayServer(t), "lgn-agent-old")

		lines := relay(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`+"\n")

		if len(lines) != 1 || !strings.Contains(lines[0], `"error"`) || !strings.Contains(lines[0], "north connect") {
			t.Errorf("replies = %q, want an error that says to run north connect", lines)
		}
	})

	t.Run("not connected answers every request with how to connect", func(t *testing.T) {
		connected(t, "", "")

		lines := relay(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`+"\n")

		if len(lines) != 1 || !strings.Contains(lines[0], "north connect") {
			t.Errorf("replies = %q, want the not-connected error", lines)
		}
	})
}

func TestLoadRemote(t *testing.T) {
	t.Run("the saved file wins over stale variables", func(t *testing.T) {
		connected(t, "https://logs.example.com", "lgn-agent-current")
		t.Setenv("LOGNORTH_URL", "https://logs.example.com")
		t.Setenv("LOGNORTH_AGENT_KEY", "lgn-agent-stale")

		r, err := loadRemote()

		if err != nil || r.Key != "lgn-agent-current" {
			t.Errorf("key = %q, err = %v, want the saved key", r.Key, err)
		}
	})

	t.Run("variables work when nothing is saved", func(t *testing.T) {
		connected(t, "", "")
		t.Setenv("LOGNORTH_URL", "https://logs.example.com")
		t.Setenv("LOGNORTH_AGENT_KEY", "lgn-agent-env")

		r, err := loadRemote()

		if err != nil || r.Key != "lgn-agent-env" {
			t.Errorf("key = %q, err = %v, want the key from the environment", r.Key, err)
		}
	})
}

func TestCall(t *testing.T) {
	s, c := testServer(t)
	s.apps = []app{{ID: 1, Name: "checkout-prod"}}
	connected(t, c.URL, testKey)

	stdout := os.Stdout
	read, write, _ := os.Pipe()
	os.Stdout = write
	err := callTool([]string{"list_apps"})
	write.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(read)

	if err != nil || !strings.Contains(string(out), `"checkout-prod"`) {
		t.Errorf("output = %q, err = %v, want the tool's JSON answer", out, err)
	}
}
