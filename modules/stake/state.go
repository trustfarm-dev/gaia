package stake

import (
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/state"
	abci "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/go-wire"
)

// transfer coins
type transferFn func(from sdk.Actor, to sdk.Actor, coins coin.Coins) abci.Result

// default transfer runs full DeliverTX
func defaultTransferFn(ctx sdk.Context, store state.SimpleDB, dispatch sdk.Deliver) transferFn {
	return func(sender, receiver sdk.Actor, coins coin.Coins) (res abci.Result) {
		// Move coins from the delegator account to the pubKey lock account
		send := coin.NewSendOneTx(sender, receiver, coins)

		// If the deduction fails (too high), abort the command
		_, err := dispatch.DeliverTx(ctx, store, send)
		if err != nil {
			return abci.ErrInsufficientFunds.AppendLog(err.Error())
		}
		return
	}
}

// nolint
const (
	// Keys for store prefixes
	CandidateKey      = iota
	CandidatesListKey = iota
	DelegatorBondKey
	DelegatorBondsListKey
	ParamKey
)

// LoadCandidates - loads the pubKey bond set
// TODO ultimately this function should be made unexported... being used right now
// for patchwork of tick functionality therefor much easier if exported until
// the new SDK is created
func LoadCandidates(store state.SimpleDB) (candidates Candidates) {
	b := store.Get([]byte{CandidateKey})
	if b == nil {
		return
	}
	err := wire.ReadBinaryBytes(b, &candidates)
	if err != nil {
		panic(err) // This error should never occure big problem if does
	}
	return
}

func saveCandidates(store state.SimpleDB, candidates Candidates) {
	b := wire.BinaryBytes(candidates)
	store.Set([]byte{CandidateKey}, b)
}

/////////////////////////////////////////////////////////////////////////////////

func getDelegatorBondKey(delegator sdk.Actor, candidate crypto.PubKey) []byte {
	bondBytes := append(wire.BinaryBytes(&delegator), candidate.Bytes()...)
	return append([]byte{DelegatorBondKey}, bondBytes...)
}

//func getDelegatorFromKey(key []byte) (delegator sdk.Actor) {
//err := wire.ReadBinaryBytes(key[1:], &delegator)
//if err != nil {
//panic(fmt.Sprintf("%v", key))
//panic(err)
//}
//return
//}

func loadDelegatorBond(store state.SimpleDB,
	delegator sdk.Actor, candidate crypto.PubKey) (bond DelegatorBond) {

	delegatorBytes := store.Get(getDelegatorBondKey(delegator, candidate))
	if delegatorBytes == nil {
		return
	}

	err := wire.ReadBinaryBytes(delegatorBytes, &bond)
	if err != nil {
		panic(err)
	}
	return
}

func loadDelegatorBonds(store state.SimpleDB,
	delegator sdk.Actor) (bonds DelegatorBonds) {

	delegatorBytes := store.Get(getDelegatorBondsKey(delegator))
	if delegatorBytes == nil {
		return
	}

	err := wire.ReadBinaryBytes(delegatorBytes, &bonds)
	if err != nil {
		panic(err)
	}
	return
}

func saveDelegatorBonds(store state.SimpleDB, delegator sdk.Actor, bonds DelegatorBonds) {
	bondsBytes := wire.BinaryBytes(bonds)
	store.Set(getDelegatorBondsKey(delegator), bondsBytes)
}

func removeDelegatorBonds(store state.SimpleDB, delegator sdk.Actor) {
	store.Remove(getDelegatorBondsKey(delegator))
}

/////////////////////////////////////////////////////////////////////////////////

// load/save the global staking params
func loadParams(store state.SimpleDB) (params Params) {
	b := store.Get([]byte{ParamKey})
	if b == nil {
		return defaultParams()
	}
	err := wire.ReadBinaryBytes(b, &params)
	if err != nil {
		panic(err) // This error should never occure big problem if does
	}
	return
}
func saveParams(store state.SimpleDB, params Params) {
	b := wire.BinaryBytes(params)
	store.Set([]byte{ParamKey}, b)
}
