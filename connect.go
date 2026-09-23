package main

// north reads a LogNorth server over its MCP endpoint with a read-only agent
// key: the same tools an agent uses, so it adds no API of its own.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// remote is where the laptop commands read from.
type remote struct {
	URL  string `json:"url"`
	Key  string `json:"key"`
	From string `json:"-"` // where it came from: the environment or the file
}

// remotePath is ~/.config/lognorth/remote.json, or under $XDG_CONFIG_HOME.
func remotePath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "lognorth", "remote.json"), nil
}

// loadRemote reads LOGNORTH_URL and LOGNORTH_AGENT_KEY first, the same
// variables the agent plugin uses, then the file north connect saved.
func loadRemote() (remote, error) {
	if fromEnv() {
		return remote{URL: normalizeURL(os.Getenv("LOGNORTH_URL")), Key: os.Getenv("LOGNORTH_AGENT_KEY"), From: "LOGNORTH_URL"}, nil
	}

	path, err := remotePath()
	if err != nil {
		return remote{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return remote{}, errors.New("not connected yet. Run north connect: it asks for your LogNorth URL and agent key")
	}
	if err != nil {
		return remote{}, err
	}
	var r remote
	if err := json.Unmarshal(data, &r); err != nil {
		return remote{}, fmt.Errorf("could not read %s: %w", path, err)
	}
	r.From = path
	return r, nil
}

// hostOf is the URL without its scheme: logs.yoursite.com.
func hostOf(url string) string {
	return strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
}

// fromShell says so when the environment, not north connect, chose the server.
func fromShell(r remote) string {
	if r.From == "LOGNORTH_URL" {
		return " (from LOGNORTH_URL in your shell)"
	}
	return ""
}

func fromEnv() bool {
	return os.Getenv("LOGNORTH_URL") != "" && os.Getenv("LOGNORTH_AGENT_KEY") != ""
}

func saveRemote(r remote) (string, error) {
	path, err := remotePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	// The key reads your production logs, so only you may read the file.
	return path, os.WriteFile(path, append(data, '\n'), 0o600)
}

// normalizeURL accepts what people paste: a trailing slash or the /mcp path.
func normalizeURL(url string) string {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	url = strings.TrimSuffix(url, "/mcp")
	if !strings.Contains(url, "://") {
		scheme := "https://"
		if strings.HasPrefix(url, "localhost") || strings.HasPrefix(url, "127.0.0.1") {
			scheme = "http://" // a LogNorth on this machine has no TLS
		}
		url = scheme + url
	}
	return url
}

// connect checks the URL and key against the server, then saves them. It
// asks for whatever the arguments leave out, and hides the key as you paste it.
func connect(args []string) error {
	if len(args) > 2 {
		return errors.New("usage: north connect [url] [agent key]")
	}
	current, _ := loadRemote() // offered as the default URL
	for len(args) < 2 {
		answer, err := ask(len(args), current.URL)
		if err != nil {
			return err
		}
		args = append(args, answer)
	}
	r := remote{URL: normalizeURL(args[0]), Key: strings.TrimSpace(args[1])}
	if !strings.HasPrefix(r.Key, "lgn-agent-") {
		if strings.HasPrefix(r.Key, "lgn-") {
			return errors.New("that is an app key, which sends events. Use the agent key from Settings > Developer: it starts with lgn-agent-")
		}
		return errors.New("an agent key starts with lgn-agent-. Find it in LogNorth under Settings > Developer")
	}

	apps, err := newClient(r).apps()
	if err != nil {
		return err
	}
	path, err := saveRemote(r)
	if err != nil {
		return fmt.Errorf("could not save the connection: %w", err)
	}

	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
	}
	fmt.Printf("Connected to %s. Apps: %s.\n", hostOf(r.URL), strings.Join(names, ", "))
	fmt.Printf("Saved to %s. Now run: north tail, or north top\n", path)
	if fromEnv() && normalizeURL(os.Getenv("LOGNORTH_URL")) != r.URL {
		fmt.Printf("Note: LOGNORTH_URL in your shell points at %s, and it wins over this file. Unset it to read %s.\n",
			normalizeURL(os.Getenv("LOGNORTH_URL")), r.URL)
	}
	return nil
}

// ask prompts for the URL (step 0) or the agent key (step 1). An empty URL
// answer keeps the server you are connected to.
func ask(step int, currentURL string) (string, error) {
	in := int(os.Stdin.Fd())
	if !term.IsTerminal(in) {
		return "", errors.New("usage: north connect <url> <agent key>. The agent key is in LogNorth under Settings > Developer")
	}
	if step == 0 {
		if currentURL != "" {
			fmt.Printf("LogNorth URL [%s]: ", currentURL)
		} else {
			fmt.Print("LogNorth URL (https://logs.yoursite.com): ")
		}
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if answer := strings.TrimSpace(line); answer != "" || currentURL == "" {
			return answer, err
		}
		return currentURL, err
	}
	fmt.Print("Agent key, from Settings > Developer (hidden): ")
	key, err := term.ReadPassword(in)
	fmt.Println()
	return strings.TrimSpace(string(key)), err
}
