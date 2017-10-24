package stake

import (
	"testing"

	"github.com/stretchr/testify/assert"

	abci "github.com/tendermint/abci/types"

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

func newTxBond(amt int64) TxBond {
	return TxBond{
		Amount: coin.Coin{"fermion", amt},
	}
}

func newTxUnbond(amt int64) TxUnbond {
	return TxUnbond{
		Amount: coin.Coin{"fermion", amt},
	}
}

func TestBondTxDuplicatePubKey(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore() // for bonds
	initSender := int64(1000)
	senders, accStore := initAccounts(2, initSender) // for accounts
	sender, sender2 := senders[0], senders[1]
	holder := getHoldAccount(sender)

	bondAmount := int64(10)
	txBond := newTxBond(bondAmount)

	txBond.PubKey = []byte("pubkey1")
	got := runTxBond(store, sender, holder, dummyTransferFn(accStore), txBond)
	assert.Equal(got, abci.OK, "expected no error on runTxBond")

	// one sender can bond to different pubkeys
	txBond.PubKey = []byte("pubkey2")
	err := checkTxBond(txBond, sender, store)
	assert.Nil(err, "expected no error on checkTx")

	// execute the last tx
	got = runTxBond(store, sender, holder, dummyTransferFn(accStore), txBond)
	assert.Equal(got, abci.OK, "expected no error on runTxBond")

	// two senders cant bond to the same pubkey
	txBond.PubKey = []byte("pubkey1")
	err = checkTxBond(txBond, sender2, store)
	assert.NotNil(err, "expected error on checkTx")
}

func TestBondTxIncrements(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore() // for bonds
	initSender := int64(1000)
	senders, accStore := initAccounts(1, initSender) // for accounts
	sender := senders[0]
	holder := getHoldAccount(sender)

	// just send the same txbond multiple times
	bondAmount := int64(10)
	txBond := newTxBond(bondAmount)
	for i := 0; i < 5; i++ {
		got := runTxBond(store, sender, holder, dummyTransferFn(accStore), txBond)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the accounts and the bond account have the appropriate values
		validators := LoadBonds(store)
		expectedBond := int64(i+1) * bondAmount // +1 since we send 1 at the start of loop
		expectedSender := initSender - expectedBond
		gotBonded := int64(validators[0].Tickets)
		gotHolder := accStore[string(holder.Address)]
		gotSender := accStore[string(sender.Address)]

		assert.Equal(expectedBond, gotBonded, "%v, %v", expectedBond, gotBonded)
		assert.Equal(expectedBond, gotHolder, "%v, %v", expectedBond, gotHolder)
		assert.Equal(expectedSender, gotSender, "%v, %v", expectedSender, gotSender)
	}
}

func TestUnbondTxIncrements(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore() // for bonds
	initSender := int64(0)
	senders, accStore := initAccounts(1, initSender) // for accounts
	sender := senders[0]
	holder := getHoldAccount(sender)

	// set initial bond
	initBond := int64(1000)
	accStore[string(sender.Address)] = initBond
	got := runTxBond(store, sender, holder, dummyTransferFn(accStore), newTxBond(initBond))
	assert.True(got.IsOK(), "expected initial bond tx to be ok, got %v", got)

	// just send the same txunbond multiple times
	unbondAmount := int64(10)
	txUnbond := newTxUnbond(unbondAmount)
	for i := 0; i < 5; i++ {
		got := runTxUnbond(store, sender, holder, dummyTransferFn(accStore), txUnbond)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the accounts and the bond account have the appropriate values
		validators := LoadBonds(store)
		expectedBond := initBond - int64(i+1)*unbondAmount // +1 since we send 1 at the start of loop
		expectedSender := initSender + (initBond - expectedBond)
		gotBonded := int64(validators[0].Tickets)
		gotHolder := accStore[string(holder.Address)]
		gotSender := accStore[string(sender.Address)]

		assert.Equal(expectedBond, gotBonded, "%v, %v", expectedBond, gotBonded)
		assert.Equal(expectedBond, gotHolder, "%v, %v", expectedBond, gotHolder)
		assert.Equal(expectedSender, gotSender, "%v, %v", expectedSender, gotSender)
	}
}

func TestBondTxMultipleVals(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore()
	initSender := int64(1000)
	senders, accStore := initAccounts(3, initSender)

	// bond them all
	for i, sender := range senders {
		txBond := newTxBond(int64(i))
		got := runTxBond(store, sender, getHoldAccount(sender), dummyTransferFn(accStore), txBond)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the account is bonded
		validators := LoadBonds(store)
		val := validators[i]
		balanceGot, balanceExpect := accStore[string(val.Sender.Address)], initSender-int64(i)
		assert.Equal(len(validators), i+1, "expected %d validators got %d", i+1, len(validators))
		assert.Equal(int(val.Tickets), i, "expected %d tickets, got %d", i, val.Tickets)
		assert.Equal(balanceGot, balanceExpect, "expected account to have %d, got %d", balanceExpect, balanceGot)
	}

	// unbond them all
	for i, sender := range senders {
		txUnbond := newTxUnbond(int64(i))
		got := runTxUnbond(store, sender, getHoldAccount(sender), dummyTransferFn(accStore), txUnbond)
		assert.True(got.IsOK(), "expected tx %d to be ok, got %v", i, got)

		//Check that the account is unbonded
		validators := LoadBonds(store)
		val := validators[0]
		validators.CleanupEmpty(store)
		validators = LoadBonds(store)
		balanceGot, balanceExpect := accStore[string(val.Sender.Address)], initSender
		assert.Equal(len(validators), len(senders)-(i+1), "expected %d validators got %d", len(senders)-(i+1), len(validators))
		assert.Equal(int(val.Tickets), 0, "expected %d tickets, got %d", i, val.Tickets)
		assert.Equal(balanceGot, balanceExpect, "expected account to have %d, got %d", balanceExpect, balanceGot)
	}
}
