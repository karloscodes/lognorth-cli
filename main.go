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
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "connect":
		err = connect(os.Args[2:])
	case "tail":
		err = tail(os.Args[2:])
	case "top":
		err = top(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
		return
	case "help", "--help", "-h":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: north <command>")
	fmt.Println()
	fmt.Println("  connect [url] [agent key]  Save where to read from; asks for what you leave out")
	fmt.Println("  tail [flags] [search]      Follow the log: --errors, --path /checkout, --app name, -n 20")
	fmt.Println("  top [--app name]           Endpoints, alerts, and uptime, live")
	fmt.Println("  version                    Show the version")
	fmt.Println()
	fmt.Println("The agent key is in LogNorth under Settings > Developer. Docs: https://lognorth.com/docs/features/terminal/")
}
