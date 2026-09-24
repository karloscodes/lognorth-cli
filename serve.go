package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// serveMCP is north mcp: an MCP server on stdin and stdout that forwards each
// message to the LogNorth server north connect saved. Agent plugins start it
// as a local command, so no plugin config holds the URL or the key.
func serveMCP(in io.Reader, out io.Writer) error {
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for lines.Scan() {
		msg := bytes.TrimSpace(lines.Bytes())
		if len(msg) == 0 {
			continue
		}
		// Read the file for every message, so a new north connect takes
		// effect without restarting the agent.
		var reply []byte
		if r, err := loadRemote(); err != nil {
			// Not connected: answer with the reason, so the agent can tell
			// the user what to run.
			reply = rpcError(msg, err.Error())
		} else {
			reply = newClient(r).forward(msg)
		}
		if reply != nil {
			out.Write(append(reply, '\n'))
		}
	}
	return lines.Err()
}

// forward posts one JSON-RPC message to the server and returns the reply to
// write back, or nil for a notification, which gets no reply.
func (c *client) forward(msg []byte) []byte {
	req, err := http.NewRequest(http.MethodPost, c.URL+"/mcp", bytes.NewReader(msg))
	if err != nil {
		return rpcError(msg, err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return rpcError(msg, fmt.Sprintf("could not reach %s: %v", c.URL, err))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	switch {
	case res.StatusCode == http.StatusAccepted:
		return nil // a notification: no reply
	case res.StatusCode == http.StatusUnauthorized:
		return rpcError(msg, "the server rejected the agent key. Run north connect with the key from Settings > Developer")
	case res.StatusCode == http.StatusTooManyRequests:
		return rpcError(msg, "rate limited: the server allows 60 agent calls a minute")
	case res.StatusCode != http.StatusOK:
		return rpcError(msg, fmt.Sprintf("%s answered %s", c.URL, res.Status))
	case len(bytes.TrimSpace(body)) == 0:
		return rpcError(msg, fmt.Sprintf("%s answered with nothing", c.URL))
	}

	// One message per line on stdio.
	var line bytes.Buffer
	if err := json.Compact(&line, body); err != nil {
		return rpcError(msg, fmt.Sprintf("%s did not answer like LogNorth", c.URL))
	}
	return line.Bytes()
}

// rpcError is a JSON-RPC error reply to msg, or nil when msg is a
// notification (no id), which must not get a reply.
func rpcError(msg []byte, message string) []byte {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(msg, &req) != nil || len(req.ID) == 0 {
		return nil
	}
	reply, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"error":   map[string]any{"code": -32000, "message": message},
	})
	return reply
}

// callTool is north call <tool> [json arguments]: it runs one tool and prints
// its answer as JSON. Agents without the MCP tools use it from the shell.
func callTool(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("usage: north call <tool> ['{\"json\": \"arguments\"}']")
	}
	arguments := map[string]any{}
	if len(args) == 2 {
		if err := json.Unmarshal([]byte(args[1]), &arguments); err != nil {
			return fmt.Errorf("the arguments must be a JSON object: %w", err)
		}
	}
	r, err := loadRemote()
	if err != nil {
		return err
	}
	var answer json.RawMessage
	if err := newClient(r).call(args[0], arguments, &answer); err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(answer))
	return err
}
