package cmd

import (
	"fmt"

	"github.com/yunus/hcloud-wg/internal/config"
	"github.com/yunus/hcloud-wg/internal/ssh"
	"github.com/yunus/hcloud-wg/internal/state"
)

func SSH(cfg *config.Config, st *state.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hw ssh <name>")
	}

	name := args[0]
	entry := st.FindByName(name)
	if entry == nil {
		return fmt.Errorf("server %q not found. Run 'hw list' to see active servers", name)
	}

	return ssh.Interactive(entry.PublicIP, cfg.SSH.User, cfg.SSH.KeyPath)
}
