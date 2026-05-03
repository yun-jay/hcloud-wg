package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/yunus/hcloud-wg/internal/config"
	"github.com/yunus/hcloud-wg/internal/hetzner"
	"github.com/yunus/hcloud-wg/internal/ssh"
	"github.com/yunus/hcloud-wg/internal/state"
	"github.com/yunus/hcloud-wg/internal/wireguard"
)

type sshRunnerImpl struct{}

func (s sshRunnerImpl) Run(host, user, keyPath, command string) (string, error) {
	return ssh.Run(host, user, keyPath, command)
}

func Create(cfg *config.Config, st *state.Store, args []string) error {
	serverType := cfg.Defaults.ServerType
	location := cfg.Defaults.Location
	image := cfg.Defaults.Image
	var name string

	// Parse args: first non-flag arg is the name, then flags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type":
			if i+1 < len(args) {
				serverType = args[i+1]
				i++
			}
		case "--location":
			if i+1 < len(args) {
				location = args[i+1]
				i++
			}
		case "--image":
			if i+1 < len(args) {
				image = args[i+1]
				i++
			}
		default:
			if name == "" && !strings.HasPrefix(args[i], "--") {
				name = args[i]
			}
		}
	}

	// Generate name if not provided
	if name == "" {
		b := make([]byte, 4)
		rand.Read(b)
		name = "wg-" + hex.EncodeToString(b)
	}

	// Check name isn't taken
	if st.FindByName(name) != nil {
		return fmt.Errorf("server %q already exists", name)
	}

	// Get next WG IP
	wgIP, err := st.NextWGIP(cfg.WireGuard.IPRangeStart, cfg.WireGuard.IPRangeEnd)
	if err != nil {
		return err
	}

	// Generate WG keypair
	privKey, pubKey, err := wireguard.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generating WireGuard keys: %w", err)
	}

	// Generate cloud-init
	cloudInit := wireguard.GenerateCloudInit(wireguard.CloudInitParams{
		WGPrivateKey:   privKey,
		WGIP:           wgIP,
		PeerPublicKey:  cfg.WireGuard.ServerPublicKey,
		PeerEndpoint:   fmt.Sprintf("%s:%d", cfg.WireGuard.ServerPublicIP, cfg.WireGuard.ServerPort),
		PeerAllowedIPs: cfg.WireGuard.Subnet,
	})

	// Find SSH key ID
	sshKeyIDs, err := findSSHKeyIDs(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Creating %s (%s in %s)...\n", name, serverType, location)

	client := hetzner.NewClient(cfg.HCloudToken)

	srv, err := client.CreateServer(name, serverType, image, location, cloudInit, sshKeyIDs, cfg.Defaults.Labels)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	publicIP := srv.PublicNet.IPv4.IP
	fmt.Printf("Server created (ID: %d, IP: %s). Waiting for boot...\n", srv.ID, publicIP)

	srv, err = client.WaitForRunning(srv.ID, 120*time.Second)
	if err != nil {
		return err
	}

	ssh.RemoveKnownHosts(publicIP)

	fmt.Println("Registering WireGuard peer on gateway...")
	runner := sshRunnerImpl{}
	err = wireguard.AddPeer(runner, cfg.WireGuard.ServerPublicIP, cfg.SSH.User, cfg.SSH.KeyPath, pubKey, wgIP)
	if err != nil {
		return err
	}

	// Save to state
	entry := state.ServerEntry{
		Name:        name,
		HCloudID:    srv.ID,
		PublicIP:    publicIP,
		WGIP:        wgIP,
		WGPublicKey: pubKey,
		ServerType:  srv.ServerType.Name,
		CreatedAt:   time.Now(),
	}
	if err := st.Add(entry); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Wait for WG tunnel
	fmt.Println("Waiting for WireGuard tunnel...")
	for i := 0; i < 10; i++ {
		_, err := ssh.Run(publicIP, cfg.SSH.User, cfg.SSH.KeyPath,
			fmt.Sprintf("ping -c1 -W2 %s", cfg.WireGuard.ServerInternalIP))
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Warning: WireGuard tunnel verification timed out (server may still be configuring)")
		}
		time.Sleep(3 * time.Second)
	}

	// Print summary
	fmt.Printf("\nServer ready:\n")
	fmt.Printf("  Name:      %s\n", name)
	fmt.Printf("  Public IP: %s\n", publicIP)
	fmt.Printf("  WG IP:     %s\n", wgIP)
	fmt.Printf("  Type:      %s (%d vCPU, %.0fGB RAM)\n", srv.ServerType.Name, srv.ServerType.Cores, srv.ServerType.Memory)
	fmt.Println()
	fmt.Printf("  SSH:       hw ssh %s\n", name)
	fmt.Printf("  Run:       hw run %s <command>\n", name)
	fmt.Printf("  Destroy:   hw destroy %s\n", name)

	return nil
}

func findSSHKeyIDs(cfg *config.Config) ([]int, error) {
	pubKeyPath := cfg.SSH.KeyPath + ".pub"

	// Get local fingerprint
	out, err := exec.Command("ssh-keygen", "-lf", pubKeyPath, "-E", "md5").Output()
	if err != nil {
		return nil, fmt.Errorf("reading SSH public key fingerprint: %w", err)
	}

	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected ssh-keygen output: %s", string(out))
	}
	localFingerprint := strings.TrimPrefix(parts[1], "MD5:")

	client := hetzner.NewClient(cfg.HCloudToken)
	keys, err := client.ListSSHKeys()
	if err != nil {
		return nil, fmt.Errorf("listing SSH keys: %w", err)
	}

	for _, key := range keys {
		if key.Fingerprint == localFingerprint {
			return []int{key.ID}, nil
		}
	}

	return nil, fmt.Errorf("SSH key %s not found in Hetzner account. Upload it first via the Hetzner console", pubKeyPath)
}
