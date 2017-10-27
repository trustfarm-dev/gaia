package stake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	crypto "github.com/tendermint/go-crypto"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/state"
)

func TestState(t *testing.T) {
	assert := assert.New(t)

	store := state.NewMemKVStore()

	validator1 := sdk.Actor{"testChain", "testapp", []byte("addressvalidator1")}

	candidates := Candidates{
		&Candidate{
			Owner:      validator1,
			PubKey:     crypto.PubKey{},
			Shares:    9,
			Delegators: sdk.Actors{},
		}}
	var validatorNilBonds Candidates

	/////////////////////////////////////////////////////////////////////////
	// Candidates checks

	//check the empty store first
	resGet := LoadCandidates(store)
	assert.Equal(validatorNilBonds, resGet)

	//Set and retrieve a record
	saveCandidates(store, candidates)
	resGet = LoadCandidates(store)
	assert.Equal(candidates, resGet)

	//modify a records, save, and retrieve
	candidates[0].Shares = 99
	saveCandidates(store, candidates)
	resGet = LoadCandidates(store)
	assert.Equal(candidates, resGet)

}

func TestDelegatorState(t *testing.T) {
	actor := sdk.NewActor("appFooBar", []byte("bar"))
	key := getDelegatorBondsKey(actor)
	got := getDelegatorFromKey(key)
	assert.True(t, actor.Equals(got), "delegator key mechanism faulty sent in %v, got %v", actor, key)
}
