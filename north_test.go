package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const testKey = "lgn-agent-test"

// server answers MCP tool calls the way LogNorth does, from memory. The real
// server is private, so this copies the rules north depends on: search_logs
// is newest first, after_id pages oldest first with no time window, and the
// default window is one hour.
type server struct {
	apps      []app
	events    []event
	endpoints []endpointRow
	alerts    []alertRow
	uptime    *uptimeReport
}

func (s *server) add(e event) {
	e.ID = uint(len(s.events) + 1)
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	s.events = append(s.events, e)
}

func (s *server) tool(name string, args map[string]any) any {
	switch name {
	case "list_apps":
		return map[string]any{"apps": s.apps}
	case "list_endpoints":
		return map[string]any{"endpoints": s.endpoints}
	case "list_alerts":
		return map[string]any{"alerts": s.alerts, "down": []downRow{}}
	case "uptime_timeline":
		return s.uptime
	case "search_logs":
		return map[string]any{"events": s.search(args)}
	}
	return nil
}

func (s *server) search(args map[string]any) []event {
	after, _ := args["after_id"].(float64)
	limit, _ := args["limit"].(float64)
	search, _ := args["search"].(string)
	errorsOnly, _ := args["errors_only"].(bool)
	found := []event{}
	for _, e := range s.events {
		switch {
		case after > 0 && e.ID <= uint(after):
		case after == 0 && e.Timestamp.Before(time.Now().Add(-time.Hour)):
		case errorsOnly && !e.IsError:
		case search != "" && !strings.Contains(e.Message+e.Context, search):
		default:
			found = append(found, e)
		}
	}
	if after == 0 {
		sort.Slice(found, func(i, j int) bool { return found[i].Timestamp.After(found[j].Timestamp) })
	}
	if limit > 0 && len(found) > int(limit) {
		found = found[:int(limit)]
	}
	return found
}

func testServer(t *testing.T) (*server, *client) {
	t.Helper()
	s := &server{apps: []app{{ID: 1, Name: "checkout-prod"}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		text, _ := json.Marshal(s.tool(req.Params.Name, req.Params.Arguments))
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{"content": []map[string]any{{"type": "text", "text": string(text)}}},
		})
	}))
	t.Cleanup(srv.Close)
	return s, newClient(remote{URL: srv.URL, Key: testKey})
}

func TestConnect(t *testing.T) {
	t.Run("saves the server and key when the server accepts them", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("LOGNORTH_URL", "")
		_, c := testServer(t)

		err := connect([]string{c.URL + "/mcp/", testKey})

		if err != nil {
			t.Fatalf("connect failed: %v", err)
		}
		saved, err := loadRemote()
		if err != nil || saved.URL != c.URL || saved.Key != testKey {
			t.Errorf("saved = %+v (%v), want %s with the key", saved, err, c.URL)
		}
		path, _ := remotePath()
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("refuses an app key", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		_, c := testServer(t)

		err := connect([]string{c.URL, "lgn-0123abcd"})

		if err == nil || !strings.Contains(err.Error(), "app key") {
			t.Errorf("err = %v, want it to say this is an app key", err)
		}
	})

	t.Run("refuses a key the server rejects, and saves nothing", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		_, c := testServer(t)

		err := connect([]string{c.URL, "lgn-agent-wrong"})

		if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Errorf("err = %v, want the rejected key", err)
		}
		path, _ := remotePath()
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("a rejected key was saved to %s", path)
		}
	})

	t.Run("a missing key without a terminal says how to pass it, instead of waiting", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		err := connect([]string{"https://logs.example.com"})

		if err == nil || !strings.Contains(err.Error(), "usage: north connect <url> <agent key>") {
			t.Errorf("err = %v, want the usage line", err)
		}
	})

	t.Run("a URL without a scheme is https, except on this machine", func(t *testing.T) {
		for in, want := range map[string]string{
			"logs.example.com/":        "https://logs.example.com",
			"logs.example.com/mcp":     "https://logs.example.com",
			"localhost:8080":           "http://localhost:8080",
			"127.0.0.1:8080/mcp":       "http://127.0.0.1:8080",
			"http://logs.internal:80/": "http://logs.internal:80",
		} {
			if got := normalizeURL(in); got != want {
				t.Errorf("normalizeURL(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("the plugin's environment variables win over the file", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("LOGNORTH_URL", "logs.example.com/")
		t.Setenv("LOGNORTH_AGENT_KEY", "lgn-agent-env")

		r, err := loadRemote()

		if err != nil || r.URL != "https://logs.example.com" || r.Key != "lgn-agent-env" {
			t.Errorf("remote = %+v (%v), want the environment", r, err)
		}
	})
}

func TestTail(t *testing.T) {
	t.Run("prints the recent events oldest first, then each new one once", func(t *testing.T) {
		s, c := testServer(t)
		s.add(event{APIKeyID: 1, Message: "first", Timestamp: time.Now().Add(-2 * time.Minute)})
		s.add(event{APIKeyID: 1, Message: "second", Timestamp: time.Now().Add(-time.Minute)})
		tl := &tailer{c: c, filter: map[string]any{}}
		var out bytes.Buffer

		if err := tl.backlog(&out, 20); err != nil {
			t.Fatalf("backlog: %v", err)
		}
		s.add(event{APIKeyID: 1, Message: "third"})
		if err := tl.poll(&out); err != nil {
			t.Fatalf("poll: %v", err)
		}
		if err := tl.poll(&out); err != nil {
			t.Fatalf("second poll: %v", err)
		}

		got := out.String()
		first, second, third := strings.Index(got, "first"), strings.Index(got, "second"), strings.Index(got, "third")
		if first < 0 || !(first < second && second < third) {
			t.Errorf("output = %q, want first, second, third in order", got)
		}
		if strings.Count(got, "third") != 1 {
			t.Errorf("output = %q, want the new event once", got)
		}
	})

	t.Run("an event whose timestamp arrived late still prints", func(t *testing.T) {
		s, c := testServer(t)
		s.add(event{APIKeyID: 1, Message: "now"})
		tl := &tailer{c: c, filter: map[string]any{}}
		var out bytes.Buffer

		tl.backlog(&out, 20)
		s.add(event{APIKeyID: 1, Message: "late", Timestamp: time.Now().Add(-3 * time.Hour)})
		tl.poll(&out)

		if !strings.Contains(out.String(), "late") {
			t.Errorf("output = %q, want the late event", out.String())
		}
	})

	t.Run("-n 0 prints nothing old, only what arrives next", func(t *testing.T) {
		s, c := testServer(t)
		s.add(event{APIKeyID: 1, Message: "old"})
		tl := &tailer{c: c, filter: map[string]any{}}
		var out bytes.Buffer

		tl.backlog(&out, 0)
		s.add(event{APIKeyID: 1, Message: "new"})
		tl.poll(&out)

		if got := out.String(); strings.Contains(got, "old") || !strings.Contains(got, "new") {
			t.Errorf("output = %q, want only the new event", got)
		}
	})

	t.Run("stops when the server ignores after_id, instead of printing twice", func(t *testing.T) {
		tl := &tailer{lastID: 5}
		var out bytes.Buffer
		old := &server{events: []event{{ID: 3, Message: "again"}}}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			text, _ := json.Marshal(map[string]any{"events": old.events})
			json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"content": []map[string]any{{"text": string(text)}}}})
		}))
		defer srv.Close()
		tl.c = newClient(remote{URL: srv.URL, Key: testKey})

		err := tl.poll(&out)

		if _, ok := err.(fatal); !ok || !strings.Contains(err.Error(), "lognorth update") {
			t.Errorf("err = %v, want a fatal error that says to update the server", err)
		}
	})

	t.Run("a failed request shows its status, error, and code location", func(t *testing.T) {
		tl := &tailer{}
		e := event{Message: "POST /checkout 500", DurationMS: 1204, IsError: true, ErrorClass: "Stripe::CardError",
			ErrorMessage: "Your card was declined.", ErrorFile: "app/services/payment_service.rb", ErrorLine: 67,
			Context: `{"method":"POST","path":"/checkout","status":500}`}

		got := tl.format(e)

		for _, want := range []string{"POST", "/checkout", "500 · 1204ms", "Stripe::CardError: Your card was declined.", "payment_service.rb:67"} {
			if !strings.Contains(got, want) {
				t.Errorf("line = %q, want %q", got, want)
			}
		}
	})

	t.Run("indents the events of the request above", func(t *testing.T) {
		tl := &tailer{}
		tl.format(event{TraceID: "t1", Context: `{"method":"POST","path":"/checkout","status":201}`})

		got := tl.format(event{TraceID: "t1", Message: "Charging card", DurationMS: 847})

		if !strings.Contains(got, "    Charging card") {
			t.Errorf("line = %q, want it indented under its request", got)
		}
	})
}

func TestTop(t *testing.T) {
	t.Run("reads the endpoints, alerts, and uptime of one app", func(t *testing.T) {
		s, c := testServer(t)
		s.endpoints = []endpointRow{{Path: "/checkout", Total: 100, Errors: 20, ErrorRate: 20}}
		s.alerts = []alertRow{{Path: "/checkout", Kind: "error_rate"}}
		s.uptime = &uptimeReport{Status: "up", LatencyMS: 84}
		st := topState{apps: s.apps}

		snap := read(c, st, true, nil)

		if snap.err != nil {
			t.Fatalf("read: %v", snap.err)
		}
		if len(snap.endpoints) != 1 || len(snap.alerts) != 1 || snap.uptime == nil || snap.uptime.LatencyMS != 84 {
			t.Errorf("snapshot = %+v, want the endpoint, the alert, and up 84ms", snap)
		}
	})

	fixture := func() (topState, snapshot) {
		pct := 99.5
		u := &uptimeReport{Status: "up", LatencyMS: 84, Uptime24h: &pct}
		u.Bars = make([]struct {
			Checks int `json:"checks"`
			Fails  int `json:"fails"`
		}, 96)
		for i := range u.Bars {
			u.Bars[i].Checks = 15
		}
		u.Bars[40].Fails = 2
		st := topState{host: "logs.example.com", apps: []app{{ID: 1, Name: "checkout-prod"}, {ID: 2, Name: "api-prod"}}}
		s := snapshot{
			appID: 1, window: "1h", at: time.Now(), uptime: u,
			endpoints: []endpointRow{
				{Path: "/checkout", Total: 1204, Errors: 219, ErrorRate: 18.2, AvgDuration: 850},
				{Path: "/health", Total: 3000, AvgDuration: 3},
			},
			alerts: []alertRow{{Path: "/checkout", Kind: "error_rate", Since: time.Now(), Summary: "18.2% errors in the last 5 min, normally 0.9%."}},
		}
		return st, s
	}

	t.Run("draws the alert, the flagged endpoint, the uptime, and the keys", func(t *testing.T) {
		st, s := fixture()

		screen := strings.Join(renderTop(st, s, false, time.Now(), 100, 30, palette{}), "\n")

		for _, want := range []string{"top · checkout-prod (1/2) · logs.example.com", "■ up 84ms", "99.50%", "> /checkout SPIKE", "1,204", "18.2%", "a app", "q quit"} {
			if !strings.Contains(screen, want) {
				t.Errorf("screen is missing %q:\n%s", want, screen)
			}
		}
	})

	t.Run("fills the window exactly and never wider", func(t *testing.T) {
		st, s := fixture()
		st.open, st.path = true, "/checkout"
		s.errors = []event{{Timestamp: time.Now(), ErrorClass: "Stripe::CardError", ErrorMessage: strings.Repeat("declined ", 30)}}

		for _, size := range [][2]int{{60, 12}, {100, 30}, {200, 50}} {
			lines := renderTop(st, s, false, time.Now(), size[0], size[1], palette{})

			if len(lines) != size[1] {
				t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), size[1])
			}
			for _, l := range lines {
				if n := utf8.RuneCountInString(l); n > size[0] {
					t.Errorf("%dx%d: a line is %d wide: %q", size[0], size[1], n, l)
				}
			}
		}
	})
}

func TestCommandLine(t *testing.T) {
	t.Run("flags work before or after the search words", func(t *testing.T) {
		for _, args := range [][]string{{"--errors", "/checkout"}, {"/checkout", "--errors"}, {"/checkout", "-n", "5", "--errors"}} {
			fs := flag.NewFlagSet("tail", flag.ContinueOnError)
			errorsOnly := fs.Bool("errors", false, "")
			fs.Int("n", 20, "")

			words, err := parseFlags(fs, args)

			if err != nil || !*errorsOnly || strings.Join(words, " ") != "/checkout" {
				t.Errorf("%v: errors=%v words=%q err=%v, want errors on and the search /checkout", args, *errorsOnly, words, err)
			}
		}
	})

	t.Run("a typo suggests the command it meant", func(t *testing.T) {
		for typed, want := range map[string]string{"tial": "tail", "tpo": "top", "conect": "connect", "deploy": ""} {
			if got := closest(typed); got != want {
				t.Errorf("closest(%q) = %q, want %q", typed, got, want)
			}
		}
	})
}
