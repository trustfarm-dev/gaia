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
	//acc := coin.Account{}
	//// vvv this causes nil pointer ref error INSIDE of GetParsed
	//_, err := query.GetParsed(sender.Address, &acc, true) //NOTE we are not using proof queries
	//if err != nil {
	//return err
	//}
	//if acc.Coins.IsGTE(coin.Coins{tx.Bond}) {
	//return fmt.Errorf("not enough coins to bond, have %v, trying to bond %v",
	//acc.Coins, tx.Bond)
	//}

	// check to see if the pubkey or sender has been registered before,
	//  if it has been used ensure that the associated account is same
	bonds := LoadCandidates(store)
	_, bond := bonds.GetByPubKey(tx.PubKey)
	if bond != nil {
		if !bond.Owner.Equals(sender) {
			return fmt.Errorf("cannot bond to pubkey used by another validator-pubKey"+
				" PubKey %v already registered with %v validator address",
				bond.PubKey, bond.Owner)
		}
	}
	_, bond = bonds.GetByOwner(sender)
	if bond != nil {
		if !bond.PubKey.Equals(tx.PubKey) {
			return fmt.Errorf("cannot bond to sender address already associated"+
				" with another validator-pubKey with the PubKey %v",
				bond.PubKey)
		}
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

	//check if have enough tickets to unbond
	bonds, err := loadDelegatorBonds(store, sender)
	if err != nil {
		return fmt.Errorf("Error attempting to retrieve %v", err.Error())
	}
	_, bond := bonds.Get(tx.PubKey)
	if bond.Tickets < uint64(tx.Bond.Amount) {
		return fmt.Errorf("not enough bond tickets to unbond, have %v, trying to unbond %v",
			bond.Tickets, tx.Bond)
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
		ctx2 := ctx.WithPermissions(HoldAccount(sender))
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

	// Get the validator bond accounts, and bond and index for this sender
	bonds := LoadCandidates(store)
	idx, bond := bonds.GetByOwner(sender)
	if bond == nil { //if it doesn't yet exist create it
		bond = NewCandidate(sender, tx.PubKey)
		bonds = bonds.Add(bond)
		idx = len(bonds) - 1
	}

	// Move coins from the sender account to a (self-bond) delegator account
	txBond := TxBond{tx.BondUpdate}
	res = runTxBond(store, sender, transferFn, txBond)
	if res.IsErr() {
		return res
	}

	// Also update the  holder account
	res = transferFn(sender, bond.HoldAccount(), coin.Coins{tx.Bond})
	if res.IsErr() {
		return res
	}

	// Update the bond and save to store
	bonds[idx].Tickets += uint64(tx.Bond.Amount)
	saveCandidates(store, bonds)

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
	delegatorBonds, err := loadDelegatorBonds(store, sender)
	if err != nil {
		return resErrLoadingDelegators(err)
	}
	if len(delegatorBonds) == 0 {
		delegatorBonds = DelegatorBonds{
			&DelegatorBond{
				PubKey:  tx.PubKey,
				Tickets: 0,
			},
		}
	}

	// Add tickets to delegator bond and candidate
	j, _ := delegatorBonds.Get(tx.PubKey)
	delegatorBonds[j].Tickets += uint64(tx.Bond.Amount)
	candidates[i].Tickets += uint64(tx.Bond.Amount)

	// Save to store
	saveCandidates(store, candidates)
	saveDelegatorBonds(store, sender, delegatorBonds)

	return abci.OK
}

func runTxUnbond(store state.SimpleDB, sender sdk.Actor,
	transferFn transferFn, tx TxUnbond) (res abci.Result) {

	//get delegator bond
	delegatorBonds, err := loadDelegatorBonds(store, sender)
	if err != nil {
		return resErrLoadingDelegators(err)
	}
	_, delegatorBond := delegatorBonds.Get(tx.PubKey)
	if delegatorBond == nil {
		return resNoDelegatorForAddress
	}

	//get pubKey bond
	candidates := LoadCandidates(store)
	bvIndex, candidate := candidates.GetByPubKey(tx.PubKey)
	if candidate == nil {
		return resNoCandidateForAddress
	}

	// subtract bond tokens from delegatorBond
	if delegatorBond.Tickets.LT(tx.Bond.Amount) {
		return resInsufficientFunds
	}
	delegatorBond.Tickets -= tx.Bond.Amount

	if delegatorBond.Tickets.Equal(Zero) {
		//begin to unbond all of the tokens if the validator unbonds their last token
		if sender.Equals(tx.PubKey) {
			res = fullyUnbondPubKey(candidate, store)
			if res.IsErr() {
				return res //TODO add more context to this error?
			}
		} else {
			removeDelegatorBonds(store, sender)
		}
	} else {
		saveDelegatorBonds(store, sender, delegatorBonds)
	}

	// transfer coins back to account
	candidate.Tickets -= tx.Bond.Amount
	if candidate.TotalBondTokens.Equal(Zero) {
		candidates.Remove(bvIndex)
	}
	unbondCoin := tx.Bond
	unbondAmt := uint64(unbondCoin.Amount)
	res = transferFn(candidate.HoldAccount(), sender, coin.Coins{unbondCoin})
	if res.IsErr() {
		return res
	}

	bond.Tickets -= unbondAmt

	saveCandidates(store, candidates)
	return abci.OK
}

//TODO improve efficiency of this function
func fullyUnbondPubKey(candidate *Candidate, store state.SimpleDB,
	sender sdk.Actor, transferFn transferFn) (res abci.Result) {

	//TODO upgrade list queue... make sure that endByte as nil is okay
	allDelegators := store.List([]byte{delegatorKeyPrefix}, nil, maxVal)

	for _, delegatorRec := range allDelegators {

		delegator, err := getDelegatorFromKey(delegatorRec.Key)
		if err != nil {
			return resErrLoadingDelegator(delegatorRec.Key) //should never occur
		}
		delegatorBonds, err := loadDelegatorBonds(store, delegator)
		if err != nil {
			return resErrLoadingDelegators(err)
		}
		for _, delegatorBond := range delegatorBonds {
			if delegatorBond.PubKey.Equals(candidate.PubKey) {
				txUnbond := TxUnbond{BondUpdate{candidate.PubKey,
					coin.Coin{bondDenom, delegatorBond.Tickets}}}
				res = runTxUnbond(store, delegator, candidate.HolderAccount(), transferFn, txUnbond)
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
