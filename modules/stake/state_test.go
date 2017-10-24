package stake

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/state"
)

func TestState(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore()

	validator1 := sdk.Actor{"testChain", "testapp", []byte("addressvalidator1")}

	candidateBonds := CandidateBonds{
		&CandidateBond{
			Sender:       validator1,
			PubKey:       []byte{},
			Tickets: 9,
			HoldAccount:  sdk.Actor{"testChain", "testapp", []byte("addresslockedtoapp")},
		}}
	var validatorNilBonds CandidateBonds

	/////////////////////////////////////////////////////////////////////////
	// CandidateBonds checks

	//check the empty store first
	resGet := LoadBonds(store)
	assert.Equal(validatorNilBonds, resGet)

	//Set and retrieve a record
	saveBonds(store, candidateBonds)
	resGet = LoadBonds(store)
	assert.Equal(candidateBonds, resGet)

	//modify a records, save, and retrieve
	candidateBonds[0].Tickets = 99
	saveBonds(store, candidateBonds)
	resGet = LoadBonds(store)
	assert.Equal(candidateBonds, resGet)

}
