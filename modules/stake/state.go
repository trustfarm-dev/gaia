package stake

import (
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/state"
	abci "github.com/tendermint/abci/types"
	"github.com/tendermint/go-wire"
)

// transfer coins
type transferFn func(from sdk.Actor, to sdk.Actor, coins coin.Coins) abci.Result

// default transfer runs full DeliverTX
func defaultTransferFn(ctx sdk.Context, store state.SimpleDB, dispatch sdk.Deliver) transferFn {
	return func(sender, receiver sdk.Actor, coins coin.Coins) (res abci.Result) {
		// Move coins from the delegator account to the validator lock account
		send := coin.NewSendOneTx(sender, receiver, coins)

		// If the deduction fails (too high), abort the command
		_, err := dispatch.DeliverTx(ctx, store, send)
		if err != nil {
			return abci.ErrInsufficientFunds.AppendLog(err.Error())
		}
		return
	}
}

//BondKey - state key for the bond bytes
var (
	BondKey  = []byte{0x00}
	ParamKey = []byte{0x01}
)

// LoadBonds - loads the validator bond set
// TODO ultimately this function should be made unexported... being used right now
// for patchwork of tick functionality therefor much easier if exported until
// the new SDK is created
func LoadBonds(store state.SimpleDB) (candidateBonds CandidateBonds) {
	b := store.Get(BondKey)
	if b == nil {
		return
	}
	err := wire.ReadBinaryBytes(b, &candidateBonds)
	if err != nil {
		panic(err) // This error should never occure big problem if does
	}
	return
}

func saveBonds(store state.SimpleDB, candidateBonds CandidateBonds) {
	b := wire.BinaryBytes(candidateBonds)
	store.Set(BondKey, b)
}

// load/save the global staking params
func loadParams(store state.SimpleDB) (params Params) {
	b := store.Get(ParamKey)
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
	store.Set(ParamKey, b)
}
