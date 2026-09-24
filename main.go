// north reads your LogNorth server from any machine: tail follows the log,
// top shows one app live, and connect saves where to read from.
package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		status()
		return
	}

	var err error
	switch os.Args[1] {
	case "connect":
		err = connect(os.Args[2:])
	case "tail":
		err = tail(os.Args[2:])
	case "top":
		err = top(os.Args[2:])
	case "agents":
		err = agentsCommand(os.Args[2:])
	case "update":
		err = update(os.Args[2:])
	case "mcp":
		err = serveMCP(os.Stdin, os.Stdout)
	case "call":
		err = callTool(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("north", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "north: no command %q.", os.Args[1])
		if near := closest(os.Args[1]); near != "" {
			fmt.Fprintf(os.Stderr, " Did you mean north %s?", near)
		}
		fmt.Fprintln(os.Stderr, " Run north help for the list.")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// status is what a bare north prints: where it reads from, and what to run.
func status() {
	fmt.Printf("north %s: your LogNorth server, in the terminal\n\n", version)
	if r, err := loadRemote(); err == nil {
		fmt.Printf("Reading %s%s.\n\n", hostOf(r.URL), fromShell(r))
	} else {
		fmt.Print("Not connected yet. Start with: north connect\n\n")
	}
	fmt.Println("  north tail            follow the log (--errors, --path /checkout, --app name)")
	fmt.Println("  north top             endpoints, alerts, and uptime, live")
	fmt.Println("  north connect         read another server")
	fmt.Println("  north agents          add LogNorth to your coding agents")
	fmt.Println("  north update          update north")
	fmt.Println("  north help            everything else")
}

var commands = []string{"connect", "agents", "update", "tail", "top", "mcp", "call", "version", "help"}

// closest finds the command a typo meant: at most 2 letters off.
func closest(typed string) string {
	best, bestDist := "", 3
	for _, c := range commands {
		if d := distance(typed, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// distance is the Levenshtein edit distance between two short words.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func usage() {
	fmt.Println("Usage: north <command>")
	fmt.Println()
	fmt.Println("  connect [url] [agent key]  Save where to read from; asks for what you leave out")
	fmt.Println("  agents                     Add LogNorth to Claude Code, Codex, Gemini CLI, and Cursor")
	fmt.Println("  tail [flags] [search]      Follow the log: --errors, --path /checkout, --app name, -n 20")
	fmt.Println("  top [--app name]           Endpoints, alerts, and uptime, live")
	fmt.Println("  mcp                        Relay agent MCP calls to the server, over stdin/stdout")
	fmt.Println("  call <tool> [json]         Run one MCP tool and print its JSON answer")
	fmt.Println("  update                     Update north to the latest release")
	fmt.Println("  version                    Show the version")
	fmt.Println()
	fmt.Println("The agent key is in LogNorth under Settings > Developer. Docs: https://lognorth.com/docs/features/terminal/")
}
