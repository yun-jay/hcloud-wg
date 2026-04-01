package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yunus/hcloud-wg/cmd"
	"github.com/yunus/hcloud-wg/internal/config"
	"github.com/yunus/hcloud-wg/internal/state"
)

const usage = `Usage: hw <command> [args]

Commands:
  create [name] [flags]   Create a server with WireGuard auto-configured
  list                    Show all active servers
  ssh <name>              SSH into a server (interactive)
  run <name> <cmd...>     Run a command with real-time streaming output
  destroy <name>          Delete server and remove WG peer
  destroy --all           Delete all managed servers`

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	home, _ := os.UserHomeDir()
	statePath := filepath.Join(home, ".config", "hw", "servers.json")
	st, err := state.Load(statePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "create":
		err = cmd.Create(cfg, st, args)
	case "list":
		err = cmd.List(cfg, st, args)
	case "ssh":
		err = cmd.SSH(cfg, st, args)
	case "run":
		err = cmd.Run(cfg, st, args)
	case "destroy":
		err = cmd.Destroy(cfg, st, args)
	default:
		fmt.Printf("Unknown command: %s\n\n%s\n", command, usage)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
