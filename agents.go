package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

// The plugin repo: the skill, the commands, and an MCP config that starts
// north mcp. Each agent installs it with its own plugin command.
const (
	pluginRepo  = "karloscodes/lognorth-releases"
	marketplace = "karloscodes"
)

// agent is a coding agent north can add LogNorth to.
type agent struct {
	Name  string
	found func() bool
	add   func() (string, error) // returns what it added, for the summary
}

var agents = []agent{
	{"Claude Code", onPath("claude"), addToClaude},
	{"Codex", onPath("codex"), addToCodex},
	{"Gemini CLI", onPath("gemini"), addToGemini},
}

func onPath(name string) func() bool {
	return func() bool {
		_, err := exec.LookPath(name)
		return err == nil
	}
}

func addToClaude() (string, error) {
	err := runAll(
		[]string{"claude", "plugin", "marketplace", "add", pluginRepo},
		[]string{"claude", "plugin", "marketplace", "update", marketplace},
		[]string{"claude", "plugin", "install", "lognorth@" + marketplace},
		[]string{"claude", "plugin", "update", "lognorth@" + marketplace},
	)
	return "plugin lognorth", err
}

func addToCodex() (string, error) {
	err := runAll(
		[]string{"codex", "plugin", "marketplace", "add", pluginRepo},
		[]string{"codex", "plugin", "marketplace", "upgrade", marketplace},
		[]string{"codex", "plugin", "add", "lognorth@" + marketplace},
	)
	return "plugin lognorth", err
}

func addToGemini() (string, error) {
	// install fails when the extension is there already, so update it instead.
	err := run("gemini", "extensions", "install", "https://github.com/"+pluginRepo, "--ref", "main", "--consent", "--skip-settings")
	if err != nil && strings.Contains(err.Error(), "already installed") {
		err = run("gemini", "extensions", "update", "lognorth")
	}
	return "extension lognorth", err
}

func runAll(commands ...[]string) error {
	for _, c := range commands {
		if err := run(c[0], c[1:]...); err != nil {
			return err
		}
	}
	return nil
}

// run runs one agent command quietly; its output becomes the error when it
// fails. An agent that stops to ask a question would hang north connect, so
// it gets a minute and no stdin.
func run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("%s %s: no answer after a minute. Run it yourself to see what it asks", name, strings.Join(args, " "))
	}
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), lastLine(string(out), err))
	}
	return nil
}

func lastLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last
	}
	return err.Error()
}

// setupAgents adds LogNorth to the coding agents on this machine. It asks
// first when a person is at the terminal; ask=false adds without asking.
func setupAgents(ask bool) error {
	var found []agent
	for _, a := range agents {
		if a.found() {
			found = append(found, a)
		}
	}
	if len(found) == 0 {
		fmt.Println("No coding agent found. North works with Claude Code, Codex, and Gemini CLI.")
		return nil
	}

	names := make([]string, len(found))
	for i, a := range found {
		names[i] = a.Name
	}
	if ask {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			// An agent ran north connect: it has LogNorth already.
			fmt.Printf("To add LogNorth to %s, run: north agents\n", joinAnd(names))
			return nil
		}
		if !confirm(fmt.Sprintf("Add LogNorth to %s?", joinAnd(names))) {
			fmt.Println("Skipped. Add it later with: north agents")
			return nil
		}
	}

	p := colorsFor(os.Stdout)
	failed := 0
	for _, a := range found {
		what, err := a.add()
		if err != nil {
			failed++
			fmt.Printf("  %s %-12s %v\n", p.red("✗"), a.Name, err)
			continue
		}
		fmt.Printf("  %s %-12s %s\n", p.lime("✓"), a.Name, what)
	}
	if failed == len(found) {
		return errors.New("could not add LogNorth to any agent")
	}
	fmt.Println()
	fmt.Println("Restart your agent, then ask it: what is broken in production?")
	return nil
}

// agentsCommand is north agents: add LogNorth to the agents on this machine.
func agentsCommand(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: north agents")
	}
	if _, err := loadRemote(); err != nil {
		return err
	}
	return setupAgents(false)
}

// confirm asks a yes-or-no question; enter means yes.
func confirm(question string) bool {
	fmt.Printf("%s [Y/n] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "" || answer == "y" || answer == "yes"
}

func joinAnd(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
