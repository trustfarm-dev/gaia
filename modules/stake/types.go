package stake

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/state"
	abci "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
	cmn "github.com/tendermint/tmlibs/common"
)

// Params defines the high level settings for staking
type Params struct {
	MaxVals          int    `json:"max_vals"`           // maximum number of validators
	AllowedBondDenom string `json:"allowed_bond_denom"` // bondable coin denomination

	// gas costs for txs
	GasBond   uint64 `json:"gas_bond"`
	GasUnbond uint64 `json:"gas_unbond"`
}

func defaultParams() Params {
	return Params{
		MaxVals:          100,
		AllowedBondDenom: "fermion",
		GasBond:          20,
		GasUnbond:        0,
	}
}

//--------------------------------------------------------------------------------

// CandidateBond defines the total amount of bond tickets and their exchange rate to
// coins, associated with a single validator. Accumulation of interest is modelled
// as an in increase in the exchange rate, and slashing as a decrease.
// When coins are delegated to this validator, the validator is credited
// with a DelegatorBond whose number of bond tickets is based on the amount of coins
// delegated divided by the current exchange rate. Voting power can be calculated as
// total bonds multiplied by exchange rate.
type CandidateBond struct {
	Candidate   crypto.PubKey // Pubkey of validator
	Owner       sdk.Actor     // Sender of BondTx - UnbondTx returns here
	Tickets     uint64        // Total number of bond tickets for the validator
	HoldCoin    uint64        // Account where the bonded coins are held. Controlled by the app
	HoldAccount sdk.Actor     // Account where the bonded coins are held. Controlled by the app
	VotingPower uint64        // Total number of bond tickets for the validator
}

// NewCandidateBond - returns a new empty validator bond object
func NewCandidateBond(owner, holder sdk.Actor, candidate crypto.PubKey) *CandidateBond {
	return &CandidateBond{
		Owner:       owner,
		Candidate:   candidate,
		Tickets:     0,
		HoldCoin:    0,
		HoldAccount: holder,
		VotingPower: 0,
	}
}

// ABCIValidator - Get the validator from a bond value
func (vb CandidateBond) ABCIValidator() *abci.Validator {
	return &abci.Validator{
		PubKey: vb.PubKey,
		Power:  vb.VotingPower,
	}
}

//--------------------------------------------------------------------------------

// CandidateBonds - the set of all CandidateBonds
type CandidateBonds []*CandidateBond

var _ sort.Interface = CandidateBonds{} //enforce the sort interface at compile time

// nolint - sort interface functions
func (vbs CandidateBonds) Len() int      { return len(vbs) }
func (vbs CandidateBonds) Swap(i, j int) { vbs[i], vbs[j] = vbs[j], vbs[i] }
func (vbs CandidateBonds) Less(i, j int) bool {
	vp1, vp2 := vbs[i].VotingPower, vbs[j].VotingPower
	d1, d2 := vbs[i].Sender, vbs[j].Sender
	switch {
	case vp1 != vp2:
		return vp1 > vp2
	case d1.ChainID != d2.ChainID:
		return d1.ChainID < d2.ChainID
	case d1.App != d2.App:
		return d1.App < d2.App
	default:
		return bytes.Compare(d1.Address, d2.Address) == -1
	}
}

// Sort - Sort the array of bonded values
func (vbs CandidateBonds) Sort() {
	sort.Sort(vbs)
}

// UpdateVotingPower - voting power based on bond tickets and exchange rate
// TODO make not a function of CandidateBonds as CandidateBonds can be loaded from the store
func (vbs CandidateBonds) UpdateVotingPower(store state.SimpleDB) {

	for _, vb := range vbs {
		vb.VotingPower = vb.Tickets
	}

	// Now sort and truncate the power
	vbs.Sort()
	for i, vb := range vbs {
		if i >= loadParams(store).MaxVals {
			vb.VotingPower = 0
		}
	}
	saveBonds(store, vbs)
	return
}

// CleanupEmpty - removes all validators which have no bonded atoms left
func (vbs CandidateBonds) CleanupEmpty(store state.SimpleDB) {
	for i, vb := range vbs {
		if vb.Tickets == 0 {
			var err error
			vbs, err = vbs.Remove(i)
			if err != nil {
				cmn.PanicSanity(resBadRemoveValidator.Error())
			}
		}
	}
	saveBonds(store, vbs)
}

// GetValidators - get the most recent updated validator set from the
// CandidateBonds. These bonds are already sorted by VotingPower from
// the UpdateVotingPower function which is the only function which
// is to modify the VotingPower
func (vbs CandidateBonds) GetValidators(store state.SimpleDB) []*abci.Validator {
	maxVals := loadParams(store).MaxVals
	validators := make([]*abci.Validator, cmn.MinInt(len(vbs), maxVals))
	for i, vb := range vbs {
		if vb.VotingPower == 0 { //exit as soon as the first Voting power set to zero is found
			break
		}
		if i >= maxVals {
			return validators
		}
		validators[i] = vb.ABCIValidator()
	}
	return validators
}

// ValidatorsDiff - get the difference in the validator set from the input validator set
func ValidatorsDiff(previous, current []*abci.Validator, store state.SimpleDB) (diff []*abci.Validator) {

	//TODO do something more efficient possibly by sorting first

	//calculate any differences from the previous to the new validator set
	// first loop through the previous validator set, and then catch any
	// missed records in the new validator set
	diff = make([]*abci.Validator, 0, loadParams(store).MaxVals)

	for _, prevVal := range previous {
		if prevVal == nil {
			continue
		}
		found := false
		for _, curVal := range current {
			if curVal == nil {
				continue
			}
			if bytes.Equal(prevVal.PubKey, curVal.PubKey) {
				found = true
				if curVal.Power != prevVal.Power {
					diff = append(diff, &abci.Validator{curVal.PubKey, curVal.Power})
					break
				}
			}
		}
		if !found {
			diff = append(diff, &abci.Validator{prevVal.PubKey, 0})
		}
	}
	for _, curVal := range current {
		if curVal == nil {
			continue
		}
		found := false
		for _, prevVal := range previous {
			if prevVal == nil {
				continue
			}
			if bytes.Equal(prevVal.PubKey, curVal.PubKey) {
				found = true
				break
			}
		}
		if !found {
			diff = append(diff, &abci.Validator{curVal.PubKey, curVal.Power})
		}
	}
	return
}

// Get - get a CandidateBond for a specific sender from the CandidateBonds
func (vbs CandidateBonds) Get(sender sdk.Actor) (int, *CandidateBond) {
	for i, vb := range vbs {
		if vb.Sender.Equals(sender) {
			return i, vb
		}
	}
	return 0, nil
}

// GetByPubKey - get a CandidateBond for a specific validator from the CandidateBonds
func (vbs CandidateBonds) GetByPubKey(pubkey []byte) (int, *CandidateBond) {
	for i, vb := range vbs {
		if bytes.Equal(vb.PubKey, pubkey) {
			return i, vb
		}
	}
	return 0, nil
}

// Add - adds a CandidateBond
func (vbs CandidateBonds) Add(bond *CandidateBond) CandidateBonds {
	return append(vbs, bond)
}

// Remove - remove validator from the validator list
func (vbs CandidateBonds) Remove(i int) (CandidateBonds, error) {
	switch {
	case i < 0:
		return vbs, fmt.Errorf("Cannot remove a negative element")
	case i >= len(vbs):
		return vbs, fmt.Errorf("Element is out of upper bound")
	default:
		return append(vbs[:i], vbs[i+1:]...), nil
	}
}

//--------------------------------------------------------------------------------

// DelegatorBond represents some bond tokens held by an account.
// It is owned by one delegator, and is associated with the voting power of one candidate.
type DelegatorBond struct {
	Candidate crypto.PubKey
	Tickets   uint64
}

// DelegatorBonds - all delegator bonds existing with multiple delegatees
type DelegatorBonds []*DelegatorBond

// Get - get a DelegateeBond for a specific validator from the DelegateeBonds
func (b DelegatorBonds) Get(candidate sdk.Actor) (int, *DelegatorBond) {
	for i, bv := range b {
		if bytes.Equal(bv.Candidate.Address, candidate.Address) &&
			bv.Candidate.ChainID == candidate.ChainID &&
			bv.Candidate.App == candidate.App {
			return i, bv
		}
	}
	return 0, nil
}

// Remove - remove candidate from the candidate list
func (b DelegatorBonds) Remove(i int) (DelegatorBonds, error) {
	switch {
	case i < 0:
		return b, fmt.Errorf("Cannot remove a negative element")
	case i >= len(b):
		return b, fmt.Errorf("Element is out of upper bound")
	default:
		return append(b[:i], b[i+1:]...), nil
	}
}
