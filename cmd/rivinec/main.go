package main

import (
	"fmt"
	"os"

	"github.com/threefoldtech/rivine/build"
	"github.com/threefoldtech/rivine/modules"
	"github.com/threefoldtech/rivine/pkg/cli"
	"github.com/threefoldtech/rivine/pkg/client"
	"github.com/threefoldtech/rivine/pkg/daemon"
	"github.com/threefoldtech/rivine/types"
)

func main() {
	// create command line client
	bchainInfo := types.DefaultBlockchainInfo()
	cliClient, err := client.NewCommandLineClient("", bchainInfo.Name, daemon.RivineUserAgent)
	if err != nil {
		build.Severe(err)
	}
	// define preRunE, as to ensure we go to a default config should it be required
	cliClient.PreRunE = func(cfg *client.Config) (*client.Config, error) {
		if cfg == nil {
			chainConstants := types.StandardnetChainConstants()
			daemonConstants := modules.NewDaemonConstants(bchainInfo, chainConstants)
			newCfg := client.ConfigFromDaemonConstants(daemonConstants)
			cfg = &newCfg
		}

		if !(cfg.NetworkName == "standard" || cfg.NetworkName == "devnet" || cfg.NetworkName == "testnet") {
			return nil, fmt.Errorf("Netork name %q not recognized", cfg.NetworkName)
		}

		return cfg, nil
	}
	// start cli
	if err := cliClient.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "client exited with an error: ", err)
		// Since no commands return errors (all commands set Command.Run instead of
		// Command.RunE), Command.Execute() should only return an error on an
		// invalid command or flag. Therefore Command.Usage() was called (assuming
		// Command.SilenceUsage is false) and we should exit with exitCodeUsage.
		os.Exit(cli.ExitCodeUsage)
	}
}
