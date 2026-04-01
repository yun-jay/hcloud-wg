package wireguard

import (
	"fmt"
	"os/exec"
	"strings"
)

type SSHRunner interface {
	Run(host, user, keyPath, command string) (string, error)
}

func GenerateKeyPair() (privateKey, publicKey string, err error) {
	genKey := exec.Command("wg", "genkey")
	privOut, err := genKey.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg genkey: %w", err)
	}
	privateKey = strings.TrimSpace(string(privOut))

	pubKey := exec.Command("wg", "pubkey")
	pubKey.Stdin = strings.NewReader(privateKey)
	pubOut, err := pubKey.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey: %w", err)
	}
	publicKey = strings.TrimSpace(string(pubOut))

	return privateKey, publicKey, nil
}

type CloudInitParams struct {
	WGPrivateKey   string
	WGIP           string
	PeerPublicKey  string
	PeerEndpoint   string
	PeerAllowedIPs string
}

func GenerateCloudInit(params CloudInitParams) string {
	return fmt.Sprintf(`#cloud-config
package_update: true
packages:
  - wireguard

write_files:
  - path: /etc/wireguard/wg0.conf
    content: |
      [Interface]
      PrivateKey = %s
      Address = %s/24

      [Peer]
      PublicKey = %s
      Endpoint = %s
      AllowedIPs = %s
      PersistentKeepalive = 25

runcmd:
  - systemctl enable wg-quick@wg0
  - wg-quick up wg0
  - touch /opt/.wg-ready
`, params.WGPrivateKey, params.WGIP, params.PeerPublicKey, params.PeerEndpoint, params.PeerAllowedIPs)
}

func AddPeer(runner SSHRunner, host, user, keyPath, publicKey, allowedIP string) error {
	cmd := fmt.Sprintf("wg set wg0 peer %s allowed-ips %s/32", publicKey, allowedIP)
	_, err := runner.Run(host, user, keyPath, cmd)
	if err != nil {
		return fmt.Errorf("adding WireGuard peer: %w", err)
	}
	return nil
}

func RemovePeer(runner SSHRunner, host, user, keyPath, publicKey string) error {
	cmd := fmt.Sprintf("wg set wg0 peer %s remove", publicKey)
	_, err := runner.Run(host, user, keyPath, cmd)
	if err != nil {
		return fmt.Errorf("removing WireGuard peer: %w", err)
	}
	return nil
}
