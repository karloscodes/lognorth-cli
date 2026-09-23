package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// errRateLimited means the server allows no more agent calls this minute.
var errRateLimited = errors.New("rate limited")

type client struct {
	remote
	http *http.Client
}

func newClient(r remote) *client {
	return &client{remote: r, http: &http.Client{Timeout: 10 * time.Second}}
}

// call runs one MCP tool and decodes its JSON answer into out.
func (c *client) call(tool string, args map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	})
	req, err := http.NewRequest(http.MethodPost, c.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", c.URL, err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return fatal{errors.New("the server rejected the agent key. Run north connect again with a key from Settings > Developer")}
	case res.StatusCode == http.StatusTooManyRequests:
		return errRateLimited
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%s answered %s", c.URL, res.Status)
	}

	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rpc); err != nil {
		return fmt.Errorf("%s did not answer like LogNorth: %w", c.URL, err)
	}
	if rpc.Error != nil {
		return errors.New(rpc.Error.Message)
	}
	if len(rpc.Result.Content) == 0 {
		return fmt.Errorf("%s returned nothing", tool)
	}
	text := rpc.Result.Content[0].Text
	if rpc.Result.IsError {
		if strings.HasPrefix(text, "unknown tool") {
			return fatal{errors.New("this needs a newer LogNorth. Run lognorth update on the server")}
		}
		return errors.New(text)
	}
	return json.Unmarshal([]byte(text), out)
}

type app struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func (c *client) apps() ([]app, error) {
	var out struct {
		Apps []app `json:"apps"`
	}
	err := c.call("list_apps", map[string]any{}, &out)
	return out.Apps, err
}

// findApp matches an app by name, case-insensitive, or by id.
func findApp(apps []app, want string) (app, error) {
	for _, a := range apps {
		if strings.EqualFold(a.Name, want) || fmt.Sprint(a.ID) == want {
			return a, nil
		}
	}
	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
	}
	return app{}, fmt.Errorf("no app named %q. Apps: %s", want, strings.Join(names, ", "))
}
