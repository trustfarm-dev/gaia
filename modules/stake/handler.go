package stake

import (
	"fmt"
	"strconv"

	abci "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/log"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/errors"
	"github.com/cosmos/cosmos-sdk/modules/auth"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/stack"
	"github.com/cosmos/cosmos-sdk/state"
)

//nolint
const (
	stakingModuleName = "stake"
)

// Name is the name of the modules.
func Name() string {
	return stakingModuleName
}

// Handler - the transaction processing handler
type Handler struct {
	stack.PassInitValidate
}

// NewHandler returns a new Handler with the default Params.
func NewHandler() Handler {
	return Handler{}
}

var _ stack.Dispatchable = Handler{} // enforce interface at compile time

// Name - return stake namespace
func (Handler) Name() string {
	return stakingModuleName
}

// AssertDispatcher - placeholder for stack.Dispatchable
func (Handler) AssertDispatcher() {}

// InitState - set genesis parameters for staking
func (h Handler) InitState(l log.Logger, store state.SimpleDB,
	module, key, value string, cb sdk.InitStater) (log string, err error) {
	return "", h.initState(module, key, value, store)
}

// separated for testing
func (Handler) initState(module, key, value string, store state.SimpleDB) error {
	if module != stakingModuleName {
		return errors.ErrUnknownModule(module)
	}
	params := loadParams(store)
	switch key {
	case "allowed_bond_denom":
		params.AllowedBondDenom = value
	case "max_vals",
		"gas_bond",
		"gas_unbond":
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("input must be integer, Error: %v", err.Error())
		}
		switch key {
		case "max_vals":
			params.MaxVals = i
		case "gas_bond":
			params.GasBond = uint64(i)
		case "gas_unbound":
			params.GasUnbond = uint64(i)
		}
	default:
		return errors.ErrUnknownKey(key)
	}
	saveParams(store, params)
	return nil
}

// CheckTx checks if the tx is properly structured
func (h Handler) CheckTx(ctx sdk.Context, store state.SimpleDB,
	tx sdk.Tx, _ sdk.Checker) (res sdk.CheckResult, err error) {

	err = tx.ValidateBasic()
	if err != nil {
		return res, err
	}

	// get the sender
	sender, abciRes := getTxSender(ctx)
	if abciRes.IsErr() {
		return res, abciRes
	}

	params := loadParams(store)
	// return the fee for each tx type
	switch txInner := tx.Unwrap().(type) {
	case TxDeclareCandidacy:
		return sdk.NewCheck(params.GasDeclareCandidacy, ""),
			checkTxDeclareCandidacy(txInner, sender, store)
	case TxBond:
		return sdk.NewCheck(params.GasBond, ""),
			checkTxBond(txInner, sender, store)
	case TxUnbond:
		return sdk.NewCheck(params.GasUnbond, ""),
			checkTxUnbond(txInner, sender, store)
	}
	return res, errors.ErrUnknownTxType("GTH")
}

func checkTxDeclareCandidacy(tx TxDeclareCandidacy, sender sdk.Actor, store state.SimpleDB) error {
	// TODO check the sender has enough coins to bond

	// check to see if the pubkey or sender has been registered before,
	//  if it has been used ensure that the associated account is same
	candidate := LoadCandidate(store, tx.PubKey)
	if candidate != nil {
		return fmt.Errorf("cannot bond to pubkey which is already declared candidacy"+
			" PubKey %v already registered with %v candidate address",
			candidate.PubKey, candidate.Owner)
	}

	return checkDenom(tx.BondUpdate, store)
}

func checkTxBond(tx TxBond, sender sdk.Actor, store state.SimpleDB) error {
	// TODO check the sender has enough coins to bond

	candidate := LoadCandidate(store, tx.PubKey)
	if candidate == nil { // does PubKey exist
		return fmt.Errorf("cannot delegate to non-existant PubKey %v", tx.PubKey)
	}
	return checkDenom(tx.BondUpdate, store)
}

func checkTxUnbond(tx TxUnbond, sender sdk.Actor, store state.SimpleDB) error {

	//check if have enough shares to unbond
	bond := loadDelegatorBond(store, sender, tx.PubKey)
	if bond.Shares < uint64(tx.Bond.Amount) {
		return fmt.Errorf("not enough bond shares to unbond, have %v, trying to unbond %v",
			bond.Shares, tx.Bond)
	}
	return checkDenom(tx.BondUpdate, store)
}

func checkDenom(tx BondUpdate, store state.SimpleDB) error {
	if tx.Bond.Denom != loadParams(store).AllowedBondDenom {
		return fmt.Errorf("Invalid coin denomination")
	}
	return nil
}

// DeliverTx executes the tx if valid
func (h Handler) DeliverTx(ctx sdk.Context, store state.SimpleDB,
	tx sdk.Tx, dispatch sdk.Deliver) (res sdk.DeliverResult, err error) {

	// TODO: remove redunandcy
	// also we don't need to check the res - gas is already deducted in sdk
	_, err = h.CheckTx(ctx, store, tx, nil)
	if err != nil {
		return
	}

	sender, abciRes := getTxSender(ctx)
	if abciRes.IsErr() {
		return res, abciRes
	}

	// Run the transaction
	switch _tx := tx.Unwrap().(type) {
	case TxDeclareCandidacy:
		fn := defaultTransferFn(ctx, store, dispatch)
		abciRes = runTxDeclareCandidacy(store, sender, fn, _tx)
	case TxBond:
		fn := defaultTransferFn(ctx, store, dispatch)
		abciRes = runTxBond(store, sender, fn, _tx)
	case TxUnbond:
		//context with hold account permissions
		ctx2 := ctx.WithPermissions(getHoldAccount(sender))
		fn := defaultTransferFn(ctx2, store, dispatch)
		abciRes = runTxUnbond(store, sender, fn, _tx)
	}

	res = sdk.DeliverResult{
		Data:    abciRes.Data,
		Log:     abciRes.Log,
		GasUsed: loadParams(store).GasBond,
	}

	return
}

///////////////////////////////////////////////////////////////////////////////////////////////////
// These functions assume everything has been authenticated,
// now we just perform action and save

func runTxDeclareCandidacy(store state.SimpleDB, sender sdk.Actor,
	transferFn transferFn, tx TxDeclareCandidacy) (res abci.Result) {

	// create and save the empty candidate
	bond := LoadCandidate(store, tx.PubKey)
	if bond != nil {
		return resCandidateExistsAddr
	}
	saveCandidate(store, NewCandidate(sender, tx.PubKey))

	// move coins from the sender account to a (self-bond) delegator account
	// the candidate account will be updated automatically here
	txBond := TxBond{tx.BondUpdate}
	res = runTxBond(store, sender, transferFn, txBond)
	if res.IsErr() {
		return res
	}

	return abci.OK
}

func runTxBond(store state.SimpleDB, sender sdk.Actor,
	transferFn transferFn, tx TxBond) (res abci.Result) {

	// Get the pubKey bond account
	candidate := LoadCandidate(store, tx.PubKey)
	if candidate == nil {
		return resBondNotNominated
	}

	// Move coins from the delegator account to the pubKey lock account
	res = transferFn(sender, candidate.HoldAccount(), coin.Coins{tx.Bond})
	if res.IsErr() {
		return res
	}

	// Get or create the delegator bond
	bond := loadDelegatorBond(store, sender, tx.PubKey)
	if bond == nil {
		bond = &DelegatorBond{
			PubKey: tx.PubKey,
			Shares: 0,
		}
	}

	// Add shares to delegator bond and candidate
	bond.Shares += uint64(tx.Bond.Amount)
	candidate.Shares += uint64(tx.Bond.Amount)

	// Save to store
	saveCandidate(store, candidate)
	saveDelegatorBond(store, sender, *bond)

	return abci.OK
}

func runTxUnbond(store state.SimpleDB, sender sdk.Actor,
	transferFn transferFn, tx TxUnbond) (res abci.Result) {

	//get delegator bond
	bond := loadDelegatorBond(store, sender, tx.PubKey)
	if bond == nil {
		return resNoDelegatorForAddress
	}

	//get pubKey candidate
	candidate := LoadCandidate(store, tx.PubKey)
	if candidate == nil {
		return resNoCandidateForAddress
	}

	// subtract bond tokens from bond
	if bond.Shares < uint64(tx.Bond.Amount) {
		return resInsufficientFunds
	}
	bond.Shares -= uint64(tx.Bond.Amount)

	if bond.Shares == 0 {
		//begin to unbond all of the tokens if the validator unbonds their last token
		if sender.Equals(candidate.Owner) {
			//remove from list of delegators in the candidate
			candidate.Delegators = candidate.RemoveDelegator(sender)

			//remove bond from delegator's list
			removeDelegatorBond(store, sender, tx.PubKey)

			//panic(fmt.Sprintf("debug panic: %v\n", loadDelegatorBonds(store, candidate.Owner)))
			res = fullyUnbondPubKey(candidate, store, transferFn)
			if res.IsErr() {
				return res //TODO add more context to this error?
			}
		} else {
			//remove from list of delegators in the candidate
			candidate.Delegators = candidate.RemoveDelegator(sender)

			//remove bond from delegator's list
			removeDelegatorBond(store, sender, tx.PubKey)
		}
	} else {
		saveDelegatorBond(store, sender, *bond)
	}

	// transfer coins back to account
	candidate.Shares -= uint64(tx.Bond.Amount)
	if candidate.Shares == 0 {
		removeCandidate(store, tx.PubKey)
	}
	res = transferFn(candidate.HoldAccount(), sender, coin.Coins{tx.Bond})
	if res.IsErr() {
		return res
	}
	saveCandidate(store, candidate)

	return abci.OK
}

//TODO improve efficiency of this function
func fullyUnbondPubKey(candidate *Candidate, store state.SimpleDB, transferFn transferFn) (res abci.Result) {

	// get global params
	params := loadParams(store)
	//maxVals := params.MaxVals
	bondDenom := params.AllowedBondDenom


	for _, delegator := range delegators {

		 := loadDelegatorBonds(store, delegator)
		for _, bond := range bonds {
			if bond.PubKey.Equals(candidate.PubKey) {
				txUnbond := TxUnbond{BondUpdate{candidate.PubKey,
					coin.Coin{bondDenom, int64(bond.Shares)}}}
				res = runTxUnbond(store, delegator, transferFn, txUnbond)
				if res.IsErr() {
					return res
				}
			}
		}
	}
	return abci.OK
}

// get the sender from the ctx and ensure it matches the tx pubkey
func getTxSender(ctx sdk.Context) (sender sdk.Actor, res abci.Result) {
	senders := ctx.GetPermissions("", auth.NameSigs)
	if len(senders) != 1 {
		return sender, resMissingSignature
	}
	// TODO: ensure senders[0] matches tx.pubkey ...
	// NOTE on TODO..  right now the PubKey doesn't need to match the sender
	// and we actually don't have the means to construct the priv_validator.json
	// with its private key with current keys tooling in SDK so needs to be
	// a second key... This is still secure because you will only be able to
	// unbond to the first married account, although, you could hypotheically
	// bond some coins to somebody elses account (effectively giving them coins)
	// maybe that is worth checking more. Validators should probably be allowed
	// to use two different keys, one for validating and one with coins on it...
	// so this point may never be relevant
	return senders[0], abci.OK
}
