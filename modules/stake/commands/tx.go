package commands

import (
	"encoding/hex"
	"fmt"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	crypto "github.com/tendermint/go-crypto"

	sdk "github.com/cosmos/cosmos-sdk"
	txcmd "github.com/cosmos/cosmos-sdk/client/commands/txs"
	"github.com/cosmos/cosmos-sdk/modules/coin"

	"github.com/cosmos/gaia/modules/stake"
)

//nolint
const (
	FlagAmount = "amount"
	FlagPubKey = "pubkey"
)

//nolint
var (
	CmdDeclareCandidacy = &cobra.Command{
		Use:   "declare-candidacy",
		Short: "create new validator pubKey account and delegate some coins to it",
		RunE:  cmdDeclareCandidacy,
	}
	CmdBond = &cobra.Command{
		Use:   "bond",
		Short: "delegate coins to an existing pubKey bond account",
		RunE:  cmdBond,
	}
	CmdUnbond = &cobra.Command{
		Use:   "unbond",
		Short: "unbond coins from a pubKey bond account",
		RunE:  cmdUnbond,
	}
)

func init() {
	//Add Flags
	fsDelegation := flag.NewFlagSet("", flag.ContinueOnError)
	fsDelegation.String(FlagAmount, "1atom", "Amount of Atoms")
	fsDelegation.String(FlagPubKey, "", "PubKey of the Validator")

	CmdDeclareCandidacy.Flags().AddFlagSet(fsDelegation)
	CmdBond.Flags().AddFlagSet(fsDelegation)
	CmdUnbond.Flags().AddFlagSet(fsDelegation)
}

type makeTx func(coin.Coin, crypto.PubKey) sdk.Tx

func cmdDeclareCandidacy(cmd *cobra.Command, args []string) error {
	return cmdBondUpdate(cmd, args, stake.NewTxDeclareCandidacy)
}

func cmdBond(cmd *cobra.Command, args []string) error {
	return cmdBondUpdate(cmd, args, stake.NewTxBond)
}

func cmdUnbond(cmd *cobra.Command, args []string) error {
	return cmdBondUpdate(cmd, args, stake.NewTxUnbond)
}

func cmdBondUpdate(cmd *cobra.Command, args []string, makeTx makeTx) error {
	amount, err := coin.ParseCoin(viper.GetString(FlagAmount))
	if err != nil {
		return err
	}

	// Get the pubkey
	pubkeyStr := viper.GetString(FlagPubKey)
	if len(pubkeyStr) == 0 {
		return fmt.Errorf("must use --pubkey flag")
	}
	if len(pubkeyStr) != 64 { //if len(pkBytes) != 32 {
		return fmt.Errorf("pubkey must be Ed25519 hex encoded string which is 64 characters long")
	}
	pkBytes, err := hex.DecodeString(pubkeyStr)
	if err != nil {
		return err
	}
	var pkEd crypto.PubKeyEd25519
	copy(pkEd[:], pkBytes[:])
	pubKey := pkEd.Wrap()

	tx := makeTx(amount, pubKey)
	return txcmd.DoTx(tx)
}
