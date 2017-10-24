package stake

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/state"
)

func newActors(n int) (actors []sdk.Actor) {
	for i := 0; i < n; i++ {
		actors = append(actors, sdk.Actor{
			"testChain", "testapp", []byte(fmt.Sprintf("addr%d", i))})
	}
	return
}

//NOTE PubKey is supposed to be the binaryBytes of the crypto.PubKey
// instead this is just being set the address here for testing purposes
func bondsFromActors(actors []sdk.Actor, amts []int) (bonds []*Candidate) {
	for i, a := range actors {
		bonds = append(bonds, &Candidate{
			Sender:      a,
			PubKey:      a.Address.Bytes(),
			Tickets:     uint64(amts[i]),
			HoldAccount: getHoldAccount(a),
			VotingPower: uint64(amts[i]),
		})
	}
	return

}

func TestCandidatesMaxVals(t *testing.T) {
	params := defaultParams()
	assert := assert.New(t)
	store := state.NewMemKVStore()
	actors := newActors(3)
	bonds := Candidates(bondsFromActors(actors, []int{10, 300, 123}))

	testCases := []struct {
		maxVals, expectedVals int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 3},
	}

	for _, testCase := range testCases {
		params.MaxVals = testCase.maxVals
		saveParams(store, params)
		bonds.UpdateVotingPower(store)
		assert.Equal(testCase.expectedVals, len(bonds.GetValidators(store)), "%v", bonds.GetValidators(store))
	}
}

func TestCandidatesSort(t *testing.T) {
	params := defaultParams()
	assert, require := assert.New(t), require.New(t)
	store := state.NewMemKVStore()

	N := 5
	actors := newActors(N)
	bonds := Candidates(bondsFromActors(actors, []int{10, 300, 123, 4, 200}))
	expectedOrder := []int{1, 4, 2, 0, 3}

	//test basic sort
	bonds.Sort()

	vals := bonds.GetValidators(store)
	require.Equal(N, len(vals))

	for i, val := range vals {
		expectedIdx := expectedOrder[i]
		assert.Equal(val.PubKey, actors[expectedIdx].Address.Bytes())
	}

	// now reduce the maxvals, ensure they're still ordered
	maxVals := 3
	params.MaxVals = maxVals
	saveParams(store, params)
	bonds.UpdateVotingPower(store)
	vals = bonds.GetValidators(store)
	require.Equal(maxVals, len(vals))

	for i, val := range vals {
		expectedIdx := expectedOrder[i]
		assert.Equal(val.PubKey, actors[expectedIdx].Address.Bytes())
	}
}

func TestCandidatesUpdate(t *testing.T) {
	params := defaultParams()
	assert, require := assert.New(t), require.New(t)
	store := state.NewMemKVStore()

	actors := newActors(3)
	bonds := Candidates(bondsFromActors(actors, []int{10, 300, 123}))
	bonds.Sort()

	maxVals := 2
	params.MaxVals = maxVals
	saveParams(store, params)

	// Change some of the bonded tickets, get the new validator set
	vals1 := bonds.GetValidators(store)
	bonds[2].Tickets = 1000
	bonds.UpdateVotingPower(store)
	vals2 := bonds.GetValidators(store)

	require.Equal(maxVals, len(vals2))

	expectedOrder := []int{0, 1, 2}
	for i, val := range vals2 {
		expectedIdx := expectedOrder[i]
		assert.Equal(val.PubKey, actors[expectedIdx].Address.Bytes())
	}

	// calculate the difference in the validator set from the original set
	diff := ValidatorsDiff(vals1, vals2, store)
	require.Equal(2, len(diff), "validator diff should have length 2, diff %v, val1 %v, val2 %v",
		diff, vals1, vals2)
	assert.True(diff[0].Power == 0)
	assert.True(diff[1].Power == 1000)
}
