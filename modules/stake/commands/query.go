package commands

import (
	"github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/client/commands/query"
	"github.com/cosmos/cosmos-sdk/stack"
	"github.com/cosmos/gaia/modules/stake"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//nolint
var (
	CmdQueryValidators = &cobra.Command{
		Use:   "candidates",
		Short: "Query for the validator-candidates set",
		RunE:  cmdQueryCandidates,
	}
	// TODO individual validators
	//CmdQueryValidator = &cobra.Command{
	//Use:   "validator",
	//Short: "Query a validator account",
	//RunE:  cmdQueryValidator,
	//}
)

func cmdQueryCandidates(cmd *cobra.Command, args []string) error {

	var bonds stake.Candidates

	prove := !viper.GetBool(commands.FlagTrustNode)
	key := stack.PrefixedKey(stake.Name(), []byte{stake.CandidateKey})
	h, err := query.GetParsed(key, &bonds, prove)
	if err != nil {
		return err
	}

	return query.OutputProof(bonds, h)
}
