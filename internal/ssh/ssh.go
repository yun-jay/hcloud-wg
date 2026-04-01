package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

var sshFlags = []string{
	"-o", "StrictHostKeyChecking=no",
	"-o", "UserKnownHostsFile=/dev/null",
	"-o", "LogLevel=ERROR",
}

func Run(host, user, keyPath, command string) (string, error) {
	args := append([]string{}, sshFlags...)
	args = append(args, "-i", keyPath, user+"@"+host, command)

	cmd := exec.Command("ssh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("ssh command failed: %w\noutput: %s", err, string(out))
	}
	return string(out), nil
}

func RunStreaming(host, user, keyPath, command string) error {
	args := append([]string{}, sshFlags...)
	args = append(args, "-i", keyPath, user+"@"+host, command)

	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func Interactive(host, user, keyPath string) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	args := append([]string{"ssh"}, sshFlags...)
	args = append(args, "-i", keyPath, user+"@"+host)

	return syscall.Exec(sshPath, args, os.Environ())
}

func RemoveKnownHosts(ip string) error {
	cmd := exec.Command("ssh-keygen", "-R", ip)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Run() // ignore errors — file may not exist
	return nil
}
