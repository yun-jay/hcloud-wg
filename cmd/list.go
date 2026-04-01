package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/yunus/hcloud-wg/internal/config"
	"github.com/yunus/hcloud-wg/internal/hetzner"
	"github.com/yunus/hcloud-wg/internal/state"
)

func List(cfg *config.Config, st *state.Store, args []string) error {
	client := hetzner.NewClient(cfg.HCloudToken)

	servers, err := client.ListServers("managed-by=hcloud-wg")
	if err != nil {
		return fmt.Errorf("listing servers: %w", err)
	}

	// Build map of active IDs and status
	activeIDs := make(map[int]bool)
	statusMap := make(map[int]string)
	for _, s := range servers {
		activeIDs[s.ID] = true
		statusMap[s.ID] = s.Status
	}

	// Prune stale entries
	st.Prune(activeIDs)

	entries := st.All()
	if len(entries) == 0 {
		fmt.Println("No active servers.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tWG IP\tPUBLIC IP\tTYPE\tSTATUS\tAGE")

	for _, e := range entries {
		status := statusMap[e.HCloudID]
		if status == "" {
			status = "unknown"
		}
		age := formatAge(time.Since(e.CreatedAt))
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Name, e.WGIP, e.PublicIP, e.ServerType, status, age)
	}

	w.Flush()
	return nil
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
