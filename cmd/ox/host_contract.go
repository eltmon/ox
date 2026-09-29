package main

import (
	"fmt"
	"runtime/debug"

	"github.com/sageox/ox/internal/cli"
	"github.com/sageox/ox/internal/version"
	"github.com/spf13/cobra"
)

// HostContract is the contract id a host (Overdeck) probes for before it
// wires ox into an agent launch. A binary that answers it supports
// host-managed mode (OX_HOST_MANAGED, OX_HOST_NETWORK). Upstream ox has no
// host-contract command, so the probe fails there and the host fails closed.
const HostContract = "overdeck-host/1"

type hostContractInfo struct {
	Contract    string `json:"contract"`
	HostManaged bool   `json:"hostManaged"` // this binary supports host-managed mode
	Version     string `json:"version"`
	Commit      string `json:"commit"` // vcs.revision stamped by `go build`, or ""
}

var hostContractCmd = &cobra.Command{
	Use:    "host-contract",
	Short:  "Print the host integration contract this ox supports",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		info := hostContractInfo{
			Contract:    HostContract,
			HostManaged: true,
			Version:     version.Version,
			Commit:      buildRevision(),
		}
		if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
			return cli.PrintJSONTo(cmd.OutOrStdout(), info)
		}
		commit := info.Commit
		if commit == "" {
			commit = "unknown"
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s (ox %s, commit %s)\n", info.Contract, info.Version, commit)
		return err
	},
}

// buildRevision returns the vcs.revision the Go toolchain stamped into this
// binary, or "" when the build carried no VCS information.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

func init() {
	hostContractCmd.Flags().Bool("json", false, "output the contract as JSON")
	rootCmd.AddCommand(hostContractCmd)
}
