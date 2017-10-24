//nolint
package stake

import (
	"fmt"

	abci "github.com/tendermint/abci/types"
)

var (
	errBadBondingDenom    = fmt.Errorf("Invalid coin denomination")
	errBadBondingAmount   = fmt.Errorf("Amount must be > 0")
	errNoBondingAcct      = fmt.Errorf("No bond account for this (address, validator) pair")
	errCommissionNegative = fmt.Errorf("Commission must be positive")
	errCommissionHuge     = fmt.Errorf("Commission cannot be more than 100%")

	resBadValidatorAddr      = abci.ErrBaseUnknownAddress.AppendLog("Validator does not exist for that address")
	resMissingSignature      = abci.ErrBaseInvalidSignature.AppendLog("Missing signature")
	resBondNotNominated      = abci.ErrBaseInvalidOutput.AppendLog("Cannot bond to non-nominated account")
	resNoCandidateForAddress = abci.ErrBaseUnknownAddress.AppendLog("Validator does not exist for that address")
	resNoDelegatorForAddress = abci.ErrBaseInvalidInput.AppendLog("Delegator does not contain validator bond")
	resInsufficientFunds     = abci.ErrBaseInsufficientFunds.AppendLog("Insufficient bond tickets")
	resBadRemoveValidator    = abci.ErrInternalError.AppendLog("Error removing validator")

	invalidInput = abci.CodeType_BaseInvalidInput
)

func resErrLoadingDelegators(err error) abci.Result {
	return abci.ErrBaseEncodingError.AppendLog("Error loading delegators: " + err.Error()) //should never occur
}
