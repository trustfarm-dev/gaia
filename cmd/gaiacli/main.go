package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/tendermint/tmlibs/cli"

	"github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/client/commands/auto"
	"github.com/cosmos/cosmos-sdk/client/commands/keys"
	"github.com/cosmos/cosmos-sdk/client/commands/proxy"
	"github.com/cosmos/cosmos-sdk/client/commands/query"
	rpccmd "github.com/cosmos/cosmos-sdk/client/commands/rpc"
	"github.com/cosmos/cosmos-sdk/client/commands/seeds"
	txcmd "github.com/cosmos/cosmos-sdk/client/commands/txs"
	authcmd "github.com/cosmos/cosmos-sdk/modules/auth/commands"
	basecmd "github.com/cosmos/cosmos-sdk/modules/base/commands"
	coincmd "github.com/cosmos/cosmos-sdk/modules/coin/commands"
	feecmd "github.com/cosmos/cosmos-sdk/modules/fee/commands"
	ibccmd "github.com/cosmos/cosmos-sdk/modules/ibc/commands"
	noncecmd "github.com/cosmos/cosmos-sdk/modules/nonce/commands"
	rolecmd "github.com/cosmos/cosmos-sdk/modules/roles/commands"

	stakecmd "github.com/cosmos/gaia/modules/stake/commands"
	"github.com/cosmos/gaia/version"
)

// GaiaCli represents the base command when called without any subcommands
var GaiaCli = &cobra.Command{
	Use:   "gaiacli",
	Short: "Client for Cosmos-Gaia blockchain",
}

func main() {
	commands.AddBasicFlags(GaiaCli)

	// Prepare queries
	query.RootCmd.AddCommand(
		// These are default parsers, but optional in your app (you can remove key)
		query.TxQueryCmd,
		query.KeyQueryCmd,
		coincmd.AccountQueryCmd,
		noncecmd.NonceQueryCmd,
		rolecmd.RoleQueryCmd,
		ibccmd.IBCQueryCmd,

		//stakecmd.CmdQueryValidator,
		stakecmd.CmdQueryCandidates,
		stakecmd.CmdQueryCandidate,
	)

	// set up the middleware
	txcmd.Middleware = txcmd.Wrappers{
		feecmd.FeeWrapper{},
		rolecmd.RoleWrapper{},
		noncecmd.NonceWrapper{},
		basecmd.ChainWrapper{},
		authcmd.SigWrapper{},
	}
	txcmd.Middleware.Register(txcmd.RootCmd.PersistentFlags())

	// you will always want this for the base send command
	txcmd.RootCmd.AddCommand(
		// This is the default transaction, optional in your app
		coincmd.SendTxCmd,
		coincmd.CreditTxCmd,
		// this enables creating roles
		rolecmd.CreateRoleTxCmd,
		// these are for handling ibc
		ibccmd.RegisterChainTxCmd,
		ibccmd.UpdateChainTxCmd,
		ibccmd.PostPacketTxCmd,

		stakecmd.CmdBond,
		stakecmd.CmdUnbond,
	)

	// Set up the various commands to use
	GaiaCli.AddCommand(
		commands.InitCmd,
		commands.ResetCmd,
		keys.RootCmd,
		seeds.RootCmd,
		rpccmd.RootCmd,
		query.RootCmd,
		txcmd.RootCmd,
		proxy.RootCmd,
		version.VersionCmd,
		auto.AutoCompleteCmd,
	)

	cmd := cli.PrepareMainCmd(GaiaCli, "BC", os.ExpandEnv("$HOME/.cosmos-gaia-cli"))
	cmd.Execute()
}
