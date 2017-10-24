package commands

import (
	"encoding/hex"
	"fmt"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	crypto "github.com/tendermint/go-crypto"
	wire "github.com/tendermint/go-wire"

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

func cmdBond(cmd *cobra.Command, args []string) error {
	amount, err := coin.ParseCoin(viper.GetString(FlagAmount))
	if err != nil {
		return err
	}

	// Get the pubkey
	pubkeyStr := viper.GetString(FlagPubKey)
	if len(pubkeyStr) == 0 {
		return fmt.Errorf("must use --pubkey flag")
	}
	pubKey, err := hex.DecodeString(pubkeyStr)
	if err != nil {
		return err
	}

	tx := stake.NewTxBond(amount, wire.BinaryBytes(pubkey))
	return txcmd.DoTx(tx)
}

func cmdUnbond(cmd *cobra.Command, args []string) error {
	amount, err := coin.ParseCoin(viper.GetString(FlagAmount))
	if err != nil {
		return err
	}

	var pubKey crypto.PubKey
	pubkeyStr := viper.GetString(FlagPubKey)
	if len(pubkeyStr) > 0 {
		pubKey, err := hex.DecodeString(pubkeyStr)
		if err != nil {
			return err
		}
	}

	tx := stake.NewTxUnbond(amount)
	return txcmd.DoTx(tx)
}

func getPubKey(pubkeyStr string) (pubkey crypto.PubKey, err error) {
	if len(pubkeyStr) != 64 { //if len(pkBytes) != 32 {
		err = fmt.Errorf("pubkey must be hex encoded string which is 64 characters long")
		return
	}
	var pkBytes []byte
	pkBytes, err = hex.DecodeString(pubkeyStr)
	if err != nil {
		return
	}
	var pkEd crypto.PubKeyEd25519
	copy(pkEd[:], pkBytes[:])
	pubkey = pkEd.Wrap()
	return
}
