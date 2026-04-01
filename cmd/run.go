package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yunus/hcloud-wg/internal/config"
	"github.com/yunus/hcloud-wg/internal/ssh"
	"github.com/yunus/hcloud-wg/internal/state"
)

func Run(cfg *config.Config, st *state.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: hw run <name> <command...>")
	}

	name := args[0]
	entry := st.FindByName(name)
	if entry == nil {
		return fmt.Errorf("server %q not found. Run 'hw list' to see active servers", name)
	}

	command := strings.Join(args[1:], " ")
	err := ssh.RunStreaming(entry.PublicIP, cfg.SSH.User, cfg.SSH.KeyPath, command)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}
