# hcloud-wg

Go CLI tool for managing ephemeral Hetzner Cloud servers with automatic WireGuard VPN configuration.

## What this is

A single-binary CLI that spins up Hetzner cloud servers pre-configured to join a WireGuard network. Designed for short-lived compute tasks (ML training, batch jobs) where you need servers that can reach services on a private WireGuard subnet, then get torn down.

## Commands

```
hcloud-wg create [name] [flags]   Create a server with WireGuard auto-configured
hcloud-wg list                    Show all active servers
hcloud-wg ssh <name>              SSH into a server (interactive)
hcloud-wg run <name> <cmd...>     Run a command with real-time streaming output
hcloud-wg destroy <name>          Delete server and remove WG peer
hcloud-wg destroy --all           Delete all managed servers
```

## Implementation Plan

### Repository Structure

```
hcloud-wg/
├── go.mod                       # module github.com/yunus/hcloud-wg
├── main.go                      # entry point, command dispatch
├── CLAUDE.md
│
├── internal/
│   ├── config/
│   │   └── config.go            # YAML config loading, defaults, validation
│   ├── hetzner/
│   │   └── client.go            # Hetzner Cloud API client (raw net/http)
│   ├── wireguard/
│   │   └── wireguard.go         # WG key gen, cloud-init template, peer management
│   ├── state/
│   │   └── state.go             # Local server state tracking (servers.json)
│   └── ssh/
│       └── ssh.go               # SSH operations via system ssh binary
│
└── cmd/
    ├── create.go
    ├── list.go
    ├── ssh.go
    ├── destroy.go
    └── run.go
```

### Dependencies

- `gopkg.in/yaml.v3` — YAML config parsing (only external dependency)
- Standard library for everything else: `net/http`, `encoding/json`, `os/exec`, `text/tabwriter`, `crypto/rand`
- No Hetzner SDK, no cobra, no golang.org/x/crypto/ssh

### Config File: `~/.config/hcloud-wg/config.yaml`

```yaml
hcloud_token: ""                    # or use HCLOUD_TOKEN env var (env overrides)

wireguard:
  server_public_ip: "204.168.170.107"   # Hetzner bot server public IPv4
  server_port: 51820
  server_public_key: "nOf0mWl8XqPaWqoNkoxNEj2pvtpZ2+ujaKbSFg88WXY="
  server_internal_ip: "10.0.0.1"        # bot server WG IP (has Postgres etc.)
  subnet: "10.0.0.0/24"
  ip_range_start: 3                     # first assignable IP (10.0.0.3)
  ip_range_end: 254

ssh:
  key_path: "~/.ssh/id_ed25519"
  user: "root"

defaults:
  server_type: "cpx62"
  image: "ubuntu-24.04"
  location: "hel1"
  labels:
    managed-by: "hcloud-wg"
```

### State File: `~/.config/hcloud-wg/servers.json`

```json
[
  {
    "name": "ml-worker",
    "hcloud_id": 12345678,
    "public_ip": "65.21.x.x",
    "wg_ip": "10.0.0.3",
    "wg_public_key": "abc123...",
    "server_type": "cpx62",
    "created_at": "2026-04-01T10:00:00Z"
  }
]
```

---

## Detailed Implementation

### `main.go` — Entry Point

- Parse `os.Args` with a simple switch on `os.Args[1]`. No cobra/urfave.
- Load config via `config.Load()`.
- Load state via `state.Load()`.
- Dispatch to the appropriate `cmd/` function.
- Each command function signature: `func(cfg *config.Config, st *state.Store, args []string) error`
- Print usage on unknown command or no args.

### `internal/config/config.go` — Configuration

Types:
```go
type Config struct {
    HCloudToken string          `yaml:"hcloud_token"`
    WireGuard   WireGuardConfig `yaml:"wireguard"`
    SSH         SSHConfig       `yaml:"ssh"`
    Defaults    DefaultsConfig  `yaml:"defaults"`
}

type WireGuardConfig struct {
    ServerPublicIP  string `yaml:"server_public_ip"`
    ServerPort      int    `yaml:"server_port"`
    ServerPublicKey string `yaml:"server_public_key"`
    ServerInternalIP string `yaml:"server_internal_ip"`
    Subnet          string `yaml:"subnet"`
    IPRangeStart    int    `yaml:"ip_range_start"`
    IPRangeEnd      int    `yaml:"ip_range_end"`
}

type SSHConfig struct {
    KeyPath string `yaml:"key_path"`
    User    string `yaml:"user"`
}

type DefaultsConfig struct {
    ServerType string            `yaml:"server_type"`
    Image      string            `yaml:"image"`
    Location   string            `yaml:"location"`
    Labels     map[string]string `yaml:"labels"`
}
```

Behavior:
- `Load()` reads `~/.config/hcloud-wg/config.yaml`.
- If file doesn't exist, return an error with instructions: "Run: mkdir -p ~/.config/hcloud-wg && $EDITOR ~/.config/hcloud-wg/config.yaml"
- `HCLOUD_TOKEN` env var overrides `hcloud_token` in YAML.
- Expand `~` in `ssh.key_path` using `os.UserHomeDir()`.
- Validate: token non-empty, WG server fields set, IP range valid (start < end, both in 1-254).

### `internal/hetzner/client.go` — Hetzner Cloud API

A thin HTTP client. Base URL: `https://api.hetzner.cloud/v1`.

Types:
```go
type Client struct {
    token      string
    httpClient *http.Client
}

type Server struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Status    string    `json:"status"`
    PublicNet struct {
        IPv4 struct {
            IP string `json:"ip"`
        } `json:"ipv4"`
    } `json:"public_net"`
    ServerType struct {
        Name        string `json:"name"`
        Description string `json:"description"`
        Cores       int    `json:"cores"`
        Memory      float64 `json:"memory"`
    } `json:"server_type"`
    Created time.Time `json:"created"`
    Labels  map[string]string `json:"labels"`
}

type SSHKey struct {
    ID          int    `json:"id"`
    Name        string `json:"name"`
    Fingerprint string `json:"fingerprint"`
}
```

Methods:
- `NewClient(token string) *Client`
- `CreateServer(name, serverType, image, location, userData string, sshKeyIDs []int, labels map[string]string) (*Server, error)` — POST `/servers`. Build JSON request body. Parse response, return error if `error` field present.
- `GetServer(id int) (*Server, error)` — GET `/servers/{id}`
- `DeleteServer(id int) error` — DELETE `/servers/{id}`
- `ListServers(labelSelector string) ([]Server, error)` — GET `/servers?label_selector=managed-by=hcloud-wg`. Handle pagination if needed (check `meta.pagination`).
- `ListSSHKeys() ([]SSHKey, error)` — GET `/ssh_keys`
- `WaitForRunning(id int, timeout time.Duration) (*Server, error)` — poll `GetServer` every 2s until `status == "running"`. Print dots to stderr. Return error on timeout.

All requests: `Authorization: Bearer <token>`, `Content-Type: application/json`. Parse error responses: `{"error": {"code": "...", "message": "..."}}`.

### `internal/wireguard/wireguard.go` — WireGuard Operations

**Key Generation:**
```go
func GenerateKeyPair() (privateKey, publicKey string, err error)
```
- Run `wg genkey` via `os/exec`, capture stdout as private key.
- Pipe private key into `wg pubkey`, capture stdout as public key.
- Trim whitespace from both.

**Cloud-Init Generation:**
```go
type CloudInitParams struct {
    WGPrivateKey    string
    WGIP            string // e.g., "10.0.0.3"
    PeerPublicKey   string // bot server's WG public key
    PeerEndpoint    string // e.g., "204.168.170.107:51820"
    PeerAllowedIPs  string // "10.0.0.0/24"
}

func GenerateCloudInit(params CloudInitParams) string
```
- Returns a `#cloud-config` YAML string.
- `package_update: true`
- `packages: [wireguard]`
- `write_files:` — writes `/etc/wireguard/wg0.conf` with Interface + Peer config.
- `runcmd:` — runs `wg-quick up wg0`, then `touch /opt/.wg-ready` as a ready marker.
- Use `fmt.Sprintf` with a raw string template. Do NOT use `text/template` — the cloud-init YAML is simple enough that sprintf is cleaner.

**Peer Management (via SSH to bot server):**
```go
func AddPeer(sshRunner SSHRunner, publicKey, allowedIP string) error
func RemovePeer(sshRunner SSHRunner, publicKey string) error
```
- `AddPeer`: runs `wg set wg0 peer <pubkey> allowed-ips <ip>/32` on the bot server.
- `RemovePeer`: runs `wg set wg0 peer <pubkey> remove` on the bot server.
- `SSHRunner` is an interface: `Run(host, user, keyPath, command string) (string, error)` — allows testing.

### `internal/state/state.go` — Server State

```go
type ServerEntry struct {
    Name        string    `json:"name"`
    HCloudID    int       `json:"hcloud_id"`
    PublicIP    string    `json:"public_ip"`
    WGIP        string    `json:"wg_ip"`
    WGPublicKey string    `json:"wg_public_key"`
    ServerType  string    `json:"server_type"`
    CreatedAt   time.Time `json:"created_at"`
}

type Store struct {
    path    string
    entries []ServerEntry
}
```

Methods:
- `Load(path string) (*Store, error)` — read JSON file. If file doesn't exist, return empty store (not an error).
- `Save() error` — write to `path + ".tmp"`, then `os.Rename` to `path` (atomic).
- `Add(entry ServerEntry) error` — append + save.
- `Remove(name string) (*ServerEntry, bool)` — remove by name, save, return removed entry.
- `FindByName(name string) *ServerEntry` — nil if not found.
- `NextWGIP(rangeStart, rangeEnd int) (string, error)` — scan entries, find first unused last-octet in [rangeStart, rangeEnd]. Return `fmt.Sprintf("10.0.0.%d", octet)`. Error if range exhausted.
- `Prune(activeIDs map[int]bool) int` — remove entries whose HCloudID is not in activeIDs. Save. Return count removed.
- `All() []ServerEntry`

### `internal/ssh/ssh.go` — SSH Operations

All functions shell out to the system `ssh` binary via `os/exec`. This preserves full terminal support and SSH agent integration.

Common SSH flags for all commands:
```go
var sshFlags = []string{
    "-o", "StrictHostKeyChecking=no",
    "-o", "UserKnownHostsFile=/dev/null",
    "-o", "LogLevel=ERROR",
}
```

Functions:
```go
// Run executes a command and returns its output.
func Run(host, user, keyPath, command string) (string, error)

// RunStreaming executes a command with stdout/stderr piped to the terminal in real-time.
func RunStreaming(host, user, keyPath, command string) error

// Interactive starts an interactive SSH session (replaces the current process).
func Interactive(host, user, keyPath string) error

// RemoveKnownHosts removes entries for an IP from ~/.ssh/known_hosts.
func RemoveKnownHosts(ip string) error
```

- `Run`: `exec.Command("ssh", flags..., "-i", keyPath, user+"@"+host, command)`. Capture `CombinedOutput()`.
- `RunStreaming`: same but set `cmd.Stdout = os.Stdout`, `cmd.Stderr = os.Stderr`, `cmd.Stdin = os.Stdin`. Call `cmd.Run()`.
- `Interactive`: use `syscall.Exec` to replace the Go process with ssh. This gives a real PTY.
- `RemoveKnownHosts`: `exec.Command("ssh-keygen", "-R", ip)`.

---

## Command Implementations

### `cmd/create.go`

```
hcloud-wg create [name] [--type cpx62] [--location hel1] [--image ubuntu-24.04]
```

Parse flags after the name. If name is omitted, generate one: `wg-<4-random-hex>`.

Flow:
1. Generate name if needed (use `crypto/rand` for hex).
2. Check name isn't already in state.
3. Get next available WG IP via `state.NextWGIP()`.
4. Generate WG keypair via `wireguard.GenerateKeyPair()`.
5. Generate cloud-init via `wireguard.GenerateCloudInit()`.
6. Find SSH key ID: call `hetzner.ListSSHKeys()`, match by fingerprint of `config.SSH.KeyPath + ".pub"`. Get fingerprint via `ssh-keygen -lf <path> -E md5`.
7. Print: `Creating <name> (<server_type> in <location>)...`
8. Call `hetzner.CreateServer()` with cloud-init as user_data, labels from config.
9. Print: `Server created (ID: <id>, IP: <ip>). Waiting for boot...`
10. Call `hetzner.WaitForRunning()` with 120s timeout.
11. Call `ssh.RemoveKnownHosts(publicIP)` — clear any stale host keys.
12. Print: `Registering WireGuard peer on gateway...`
13. SSH into bot server (config.WireGuard.ServerInternalIP), call `wireguard.AddPeer()`.
14. Save to state.
15. Print: `Waiting for WireGuard tunnel...`
16. Poll: SSH into new server via public IP, run `ping -c1 -W2 <server_internal_ip>`, retry up to 10 times with 3s sleep.
17. Print summary:
    ```
    Server ready:
      Name:      ml-worker
      Public IP: 65.21.x.x
      WG IP:     10.0.0.3
      Type:      cpx62 (16 vCPU, 32GB RAM)

      SSH:       hcloud-wg ssh ml-worker
      Run:       hcloud-wg run ml-worker <command>
      Destroy:   hcloud-wg destroy ml-worker
    ```

### `cmd/list.go`

```
hcloud-wg list
```

Flow:
1. Fetch servers from Hetzner API: `hetzner.ListServers("managed-by=hcloud-wg")`.
2. Build map of active Hetzner IDs.
3. Call `state.Prune(activeIDs)` — removes local entries for servers that no longer exist.
4. If no entries, print "No active servers." and return.
5. Print table using `text/tabwriter`:
   ```
   NAME        WG IP       PUBLIC IP       TYPE    STATUS    AGE
   ml-worker   10.0.0.3    65.21.x.x       cpx62   running   25m
   batch-01    10.0.0.4    95.217.x.x      cx23    running   2h
   ```
6. Format AGE: use `time.Since()`, display as `Xs`, `Xm`, `Xh`, or `Xd`.

### `cmd/ssh.go`

```
hcloud-wg ssh <name>
```

Flow:
1. Find server in state by name. If not found: `Error: server "<name>" not found. Run 'hcloud-wg list' to see active servers.`
2. Call `ssh.Interactive(publicIP, config.SSH.User, config.SSH.KeyPath)`.
3. This replaces the process via `syscall.Exec` — no return.

### `cmd/destroy.go`

```
hcloud-wg destroy <name>
hcloud-wg destroy --all
```

Flow for single server:
1. Find in state by name. Error if not found.
2. Print: `Destroying <name>...`
3. SSH into bot server, call `wireguard.RemovePeer(wgPublicKey)`.
4. Call `hetzner.DeleteServer(hcloudID)`.
5. Call `ssh.RemoveKnownHosts(publicIP)`.
6. Call `state.Remove(name)`.
7. Print: `Destroyed <name> (10.0.0.3)`

Flow for `--all`:
1. Iterate all entries in state.
2. For each, run the single-server flow.
3. Print: `Destroyed N servers.`

### `cmd/run.go`

```
hcloud-wg run <name> <command...>
```

Flow:
1. Find server in state by name. Error if not found.
2. Join `args[1:]` with spaces as the command string.
3. Call `ssh.RunStreaming(publicIP, config.SSH.User, config.SSH.KeyPath, command)`.
4. If the SSH command returns an error with an exit code, exit with that same code.

---

## Build

```bash
go build -o hcloud-wg .
# optionally: mv hcloud-wg ~/go/bin/
```

## Example Usage

```bash
# Create a beefy ML training server
hcloud-wg create ml-trainer --type cpx62

# Check it's up
hcloud-wg list

# SSH in and set things up manually
hcloud-wg ssh ml-trainer

# Or run a command directly
hcloud-wg run ml-trainer "apt-get update && apt-get install -y python3-pip"

# When done
hcloud-wg destroy ml-trainer
```
