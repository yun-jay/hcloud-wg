package cmd

import (
	"fmt"

	"github.com/yunus/hcloud-wg/internal/config"
	"github.com/yunus/hcloud-wg/internal/hetzner"
	"github.com/yunus/hcloud-wg/internal/ssh"
	"github.com/yunus/hcloud-wg/internal/state"
	"github.com/yunus/hcloud-wg/internal/wireguard"
)

func Destroy(cfg *config.Config, st *state.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hw destroy <name> or hw destroy --all")
	}

	if args[0] == "--all" {
		return destroyAll(cfg, st)
	}

	return destroyOne(cfg, st, args[0])
}

func destroyOne(cfg *config.Config, st *state.Store, name string) error {
	entry := st.FindByName(name)
	if entry == nil {
		return fmt.Errorf("server %q not found. Run 'hw list' to see active servers", name)
	}

	fmt.Printf("Destroying %s...\n", name)

	// Remove WG peer from gateway
	runner := sshRunnerImpl{}
	wireguard.RemovePeer(runner, cfg.WireGuard.ServerPublicIP, cfg.SSH.User, cfg.SSH.KeyPath, entry.WGPublicKey)

	// Delete server
	client := hetzner.NewClient(cfg.HCloudToken)
	if err := client.DeleteServer(entry.HCloudID); err != nil {
		return fmt.Errorf("deleting server: %w", err)
	}

	// Clean up
	ssh.RemoveKnownHosts(entry.PublicIP)
	st.Remove(name)

	fmt.Printf("Destroyed %s (%s)\n", name, entry.WGIP)
	return nil
}

func destroyAll(cfg *config.Config, st *state.Store) error {
	entries := st.All()
	if len(entries) == 0 {
		fmt.Println("No servers to destroy.")
		return nil
	}

	for _, e := range entries {
		if err := destroyOne(cfg, st, e.Name); err != nil {
			fmt.Printf("Warning: failed to destroy %s: %v\n", e.Name, err)
		}
	}

	fmt.Printf("Destroyed %d servers.\n", len(entries))
	return nil
}
