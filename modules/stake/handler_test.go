package stake

import (
	"testing"

	"github.com/stretchr/testify/assert"

	abci "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"

	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/state"
)

func dummyTransferFn(store map[string]int64) transferFn {
	return func(from, to sdk.Actor, coins coin.Coins) abci.Result {
		store[string(from.Address)] -= int64(coins[0].Amount)
		store[string(to.Address)] += int64(coins[0].Amount)
		return abci.OK
	}
}

func initAccounts(n int, amount int64) ([]sdk.Actor, map[string]int64) {
	accStore := map[string]int64{}
	senders := newActors(n)
	for _, sender := range senders {
		accStore[string(sender.Address)] = amount
	}
	return senders, accStore
}

func newTxDeclareCandidacy(amt int64, pubKey string) TxDeclareCandidacy {
	return TxDeclareCandidacy{BondUpdate{
		PubKey: newPubKey(pubKey),
		Bond:   coin.Coin{"fermion", amt},
	}}
}

func newTxBond(amt int64, pubKey string) TxBond {
	return TxBond{BondUpdate{
		PubKey: newPubKey(pubKey),
		Bond:   coin.Coin{"fermion", amt},
	}}
}

func newTxUnbond(amt int64, pubKey string) TxUnbond {
	return TxUnbond{BondUpdate{
		PubKey: newPubKey(pubKey),
		Bond:   coin.Coin{"fermion", amt},
	}}
}

func newPubKey(pk string) crypto.PubKey {
	pkBytes := []byte(pk)
	var pkEd crypto.PubKeyEd25519
	copy(pkEd[:], pkBytes[:])
	return pkEd.Wrap()
}

func TestDuplicatesTxDeclareCandidacy(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore() // for bonds
	initSender := int64(1000)
	senders, accStore := initAccounts(2, initSender) // for accounts
	sender, sender2 := senders[0], senders[1]

	bondAmount := int64(10)
	txDeclareCandidacy := newTxDeclareCandidacy(bondAmount, "pubkey1")
	got := runTxDeclareCandidacy(store, sender, dummyTransferFn(accStore), txDeclareCandidacy)
	assert.Equal(got, abci.OK, "expected no error on runTxDeclareCandidacy")

	// one sender cannot bond to different pubkeys
	txDeclareCandidacy.PubKey = newPubKey("pubkey2")
	err := checkTxDeclareCandidacy(txDeclareCandidacy, sender, store)
	assert.NotNil(err, "expected error on checkTx")

	// two senders cant bond to the same pubkey
	txDeclareCandidacy.PubKey = newPubKey("pubkey1")
	err = checkTxDeclareCandidacy(txDeclareCandidacy, sender2, store)
	assert.NotNil(err, "expected error on checkTx")
}

func TestIncrementsTxBond(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore() // for bonds
	initSender := int64(1000)
	senders, accStore := initAccounts(1, initSender) // for accounts
	sender := senders[0]
	holder := getHoldAccount(sender)

	// first declare ca
	bondAmount := int64(10)
	txDeclareCandidacy := newTxDeclareCandidacy(bondAmount, "pubkey1")
	got := runTxDeclareCandidacy(store, sender, dummyTransferFn(accStore), txDeclareCandidacy)
	assert.True(got.IsOK(), "expected declare candidacy tx to be ok, got %v", got)
	expectedBond := bondAmount // 1 since we send 1 at the start of loop,

	// just send the same txbond multiple times
	txBond := newTxBond(bondAmount, "pubkey1")
	for i := 0; i < 5; i++ {
		got := runTxBond(store, sender, dummyTransferFn(accStore), txBond)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the accounts and the bond account have the appropriate values
		candidates := LoadCandidates(store)
		expectedBond += bondAmount
		expectedSender := initSender - expectedBond
		gotBonded := int64(candidates[0].Tickets)
		gotHolder := accStore[string(holder.Address)]
		gotSender := accStore[string(sender.Address)]
		assert.Equal(expectedBond, gotBonded, "%v, %v", expectedBond, gotBonded)
		assert.Equal(expectedBond, gotHolder, "%v, %v", expectedBond, gotHolder)
		assert.Equal(expectedSender, gotSender, "%v, %v", expectedSender, gotSender)
	}
}

func TestIncrementsTxUnbond(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore() // for bonds
	initSender := int64(0)
	senders, accStore := initAccounts(1, initSender) // for accounts
	sender := senders[0]
	holder := getHoldAccount(sender)

	// set initial bond
	initBond := int64(1000)
	accStore[string(sender.Address)] = initBond
	got := runTxDeclareCandidacy(store, sender, dummyTransferFn(accStore), newTxDeclareCandidacy(initBond, "pubkey1"))
	assert.True(got.IsOK(), "expected initial bond tx to be ok, got %v", got)

	// just send the same txunbond multiple times
	unbondAmount := int64(10)
	txUnbond := newTxUnbond(unbondAmount, "pubkey1")
	for i := 0; i < 5; i++ {
		got := runTxUnbond(store, sender, dummyTransferFn(accStore), txUnbond)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the accounts and the bond account have the appropriate values
		candidates := LoadCandidates(store)
		expectedBond := initBond - int64(i+1)*unbondAmount // +1 since we send 1 at the start of loop
		expectedSender := initSender + (initBond - expectedBond)
		gotBonded := int64(candidates[0].Tickets)
		gotHolder := accStore[string(holder.Address)]
		gotSender := accStore[string(sender.Address)]

		assert.Equal(expectedBond, gotBonded, "%v, %v", expectedBond, gotBonded)
		assert.Equal(expectedBond, gotHolder, "%v, %v", expectedBond, gotHolder)
		assert.Equal(expectedSender, gotSender, "%v, %v", expectedSender, gotSender)
	}
}

func TestMultipleTxDeclareCandidacy(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore()
	initSender := int64(1000)
	senders, accStore := initAccounts(3, initSender)
	pubkeys := []string{"pk1", "pk2", "pk3"}

	// bond them all
	for i, sender := range senders {
		txDeclareCandidacy := newTxDeclareCandidacy(10, pubkeys[i])
		got := runTxDeclareCandidacy(store, sender, dummyTransferFn(accStore), txDeclareCandidacy)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the account is bonded
		candidates := LoadCandidates(store)
		val := candidates[i]
		balanceGot, balanceExpd := accStore[string(val.Owner.Address)], initSender-10
		assert.Equal(i+1, len(candidates), "expected %d candidates got %d", i+1, len(candidates))
		assert.Equal(10, int(val.Tickets), "expected %d tickets, got %d", 10, val.Tickets)
		assert.Equal(balanceExpd, balanceGot, "expected account to have %d, got %d", balanceExpd, balanceGot)
	}

	// unbond them all
	for i, sender := range senders {
		txUnbond := newTxUnbond(10, pubkeys[i])
		got := runTxUnbond(store, sender, dummyTransferFn(accStore), txUnbond)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the account is unbonded
		candidates := LoadCandidates(store)
		candidate := candidates[0]
		candidates.CleanupEmpty(store)
		candidates = LoadCandidates(store)
		balanceGot, balanceExpd := accStore[string(candidate.Owner.Address)], initSender
		assert.Equal(len(senders)-(i+1), len(candidates), "expected %d candidates got %d", len(senders)-(i+1), len(candidates))
		assert.Equal(0, int(candidate.Tickets), "expected %d tickets, got %d", 0, candidate.Tickets)
		assert.Equal(balanceExpd, balanceGot, "expected account to have %d, got %d", balanceExpd, balanceGot)
	}
}
