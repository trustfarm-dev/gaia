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
	bond := LoadCandidate(store, tx.PubKey)
	if bond != nil {
		return fmt.Errorf("cannot bond to pubkey which is already declared candidacy"+
			" PubKey %v already registered with %v candidate address",
			bond.PubKey, bond.Owner)
	}
	_, bond = bonds.GetByOwner(sender)
	if bond != nil {
		return fmt.Errorf("cannot bond to sender address already associated"+
			" with another validator-pubKey with the PubKey %v",
			bond.PubKey)
	}

	return checkDenom(tx.BondUpdate, store)
}

func checkTxBond(tx TxBond, sender sdk.Actor, store state.SimpleDB) error {
	// TODO check the sender has enough coins to bond

	bonds := LoadCandidates(store)
	_, bond := bonds.GetByPubKey(tx.PubKey)
	if bond == nil { // does PubKey exist
		return fmt.Errorf("cannot delegate to non-existant PubKey %v", tx.PubKey)
	}
	return checkDenom(tx.BondUpdate, store)
}

func checkTxUnbond(tx TxUnbond, sender sdk.Actor, store state.SimpleDB) error {

	//check if have enough shares to unbond
	bonds := loadDelegatorBonds(store, sender)
	_, bond := bonds.Get(tx.PubKey)
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
	bonds := LoadCandidates(store)
	_, bond := bonds.GetByOwner(sender)
	if bond != nil { //if it doesn't yet exist create it
		return resCandidateExistsAddr
	}
	bond = NewCandidate(sender, tx.PubKey)
	bonds = bonds.Add(bond)
	saveCandidates(store, bonds)

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
	candidates := LoadCandidates(store)
	i, candidate := candidates.GetByPubKey(tx.PubKey)
	if candidate == nil {
		return resBondNotNominated
	}

	// Move coins from the delegator account to the pubKey lock account
	res = transferFn(sender, candidate.HoldAccount(), coin.Coins{tx.Bond})
	if res.IsErr() {
		return res
	}

	// Get or create delegator bonds
	delegatorBonds := loadDelegatorBonds(store, sender)
	if len(delegatorBonds) == 0 {
		delegatorBonds = DelegatorBonds{
			&DelegatorBond{
				PubKey:  tx.PubKey,
				Shares: 0,
			},
		}
	}

	// Add shares to delegator bond and candidate
	j, _ := delegatorBonds.Get(tx.PubKey)
	delegatorBonds[j].Shares += uint64(tx.Bond.Amount)
	candidates[i].Shares += uint64(tx.Bond.Amount)

	// Also add the delegator to running list of delegators
	l := len(candidates[i].Delegators)
	delegators := make([]sdk.Actor, l+1)
	copy(delegators[:l], candidates[i].Delegators[:])
	delegators[l] = sender
	candidates[i].Delegators = delegators

	// Save to store
	saveCandidates(store, candidates)
	saveDelegatorBonds(store, sender, delegatorBonds)

	return abci.OK
}

func runTxUnbond(store state.SimpleDB, sender sdk.Actor,
	transferFn transferFn, tx TxUnbond) (res abci.Result) {

	//get delegator bond
	delegatorBonds := loadDelegatorBonds(store, sender)
	_, delegatorBond := delegatorBonds.Get(tx.PubKey)
	if delegatorBond == nil {
		return resNoDelegatorForAddress
	}

	//get pubKey bond
	candidates := LoadCandidates(store)
	cdtIndex, candidate := candidates.GetByPubKey(tx.PubKey)
	if candidate == nil {
		return resNoCandidateForAddress
	}

	// subtract bond tokens from delegatorBond
	if delegatorBond.Shares < uint64(tx.Bond.Amount) {
		return resInsufficientFunds
	}
	delegatorBond.Shares -= uint64(tx.Bond.Amount)

	if delegatorBond.Shares == 0 {
		//begin to unbond all of the tokens if the validator unbonds their last token
		if sender.Equals(candidate.Owner) {
			//remove from list of delegators in the candidate
			candidate.Delegators = candidate.RemoveDelegator(sender)

			//remove bond from delegator's list
			removeDelegatorBonds(store, sender)

			//panic(fmt.Sprintf("debug panic: %v\n", loadDelegatorBonds(store, candidate.Owner)))
			res = fullyUnbondPubKey(candidate, store, transferFn)
			if res.IsErr() {
				return res //TODO add more context to this error?
			}
		} else {
			//remove from list of delegators in the candidate
			candidate.Delegators = candidate.RemoveDelegator(sender)

			//remove bond from delegator's list
			removeDelegatorBonds(store, sender)
		}
	} else {
		saveDelegatorBonds(store, sender, delegatorBonds)
	}

	// transfer coins back to account
	candidate.Shares -= uint64(tx.Bond.Amount)
	if candidate.Shares == 0 {
		candidates.Remove(cdtIndex)
	}
	res = transferFn(candidate.HoldAccount(), sender, coin.Coins{tx.Bond})
	if res.IsErr() {
		return res
	}
	saveCandidates(store, candidates)

	return abci.OK
}

//TODO improve efficiency of this function
func fullyUnbondPubKey(candidate *Candidate, store state.SimpleDB, transferFn transferFn) (res abci.Result) {

	// get global params
	params := loadParams(store)
	//maxVals := params.MaxVals
	bondDenom := params.AllowedBondDenom

	//TODO upgrade list queue... make sure that endByte as nil is okay
	//allDelegators := store.List([]byte{DelegatorBondKey}, nil, maxVals)
	delegators := candidate.Delegators

	for _, delegator := range delegators {

		delegatorBonds := loadDelegatorBonds(store, delegator)
		for _, delegatorBond := range delegatorBonds {
			if delegatorBond.PubKey.Equals(candidate.PubKey) {
				txUnbond := TxUnbond{BondUpdate{candidate.PubKey,
					coin.Coin{bondDenom, int64(delegatorBond.Shares)}}}
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
