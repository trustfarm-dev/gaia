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
	GasDeclareCandidacy uint64 `json:"gas_declare_candidacy"`
	GasBond             uint64 `json:"gas_bond"`
	GasUnbond           uint64 `json:"gas_unbond"`
}

func defaultParams() Params {
	return Params{
		MaxVals:             100,
		AllowedBondDenom:    "fermion",
		GasDeclareCandidacy: 20,
		GasBond:             20,
		GasUnbond:           0, //TODO verify that it is safe to have gas of zero here
	}
}

//--------------------------------------------------------------------------------

// Candidate defines the total amount of bond tickets and their exchange rate to
// coins, associated with a single validator. Accumulation of interest is modelled
// as an in increase in the exchange rate, and slashing as a decrease.
// When coins are delegated to this validator, the validator is credited
// with a DelegatorBond whose number of bond tickets is based on the amount of coins
// delegated divided by the current exchange rate. Voting power can be calculated as
// total bonds multiplied by exchange rate.
type Candidate struct {
	PubKey      crypto.PubKey // Pubkey of validator
	Owner       sdk.Actor     // Sender of BondTx - UnbondTx returns here
	Tickets     uint64        // Total number of bond tickets for the validator, equivalent to coins held in bond account
	VotingPower uint64        // Voting power if pubKey is a considered a validator
}

// NewCandidate - returns a new empty validator bond object
func NewCandidate(owner sdk.Actor, pubKey crypto.PubKey) *Candidate {
	return &Candidate{
		Owner:       owner,
		PubKey:      pubKey,
		Tickets:     0,
		VotingPower: 0,
	}
}

// ABCIValidator - Get the validator from a bond value
func (cb Candidate) ABCIValidator() *abci.Validator {
	return &abci.Validator{
		PubKey: cb.PubKey,
		Power:  cb.VotingPower,
	}
}

// HoldAccount - Get the hold account for the Candidate
func (cb Candidate) HoldAccount() sdk.Actor {
	return HoldAccount(cd.Owner)
}

// HoldAccount - the account where bonded atoms are held only accessed by protocol
func HoldAccount(owner sdk.Actor) sdk.Actor {
	holdAddr := append([]byte{0x00}, owner.Address[1:]...) //shift and prepend a zero
	return sdk.NewActor(stakingModuleName, holdAddr)
}

//--------------------------------------------------------------------------------

// Candidates - the set of all Candidates
type Candidates []*Candidate

var _ sort.Interface = Candidates{} //enforce the sort interface at compile time

// nolint - sort interface functions
func (cbs Candidates) Len() int      { return len(cbs) }
func (cbs Candidates) Swap(i, j int) { cbs[i], cbs[j] = cbs[j], cbs[i] }
func (cbs Candidates) Less(i, j int) bool {
	vp1, vp2 := cbs[i].VotingPower, cbs[j].VotingPower
	d1, d2 := cbs[i].Sender, cbs[j].Sender
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
func (cbs Candidates) Sort() {
	sort.Sort(cbs)
}

// UpdateVotingPower - voting power based on bond tickets and exchange rate
// TODO make not a function of Candidates as Candidates can be loaded from the store
func (cbs Candidates) UpdateVotingPower(store state.SimpleDB) {

	for _, cb := range cbs {
		cb.VotingPower = cb.Tickets
	}

	// Now sort and truncate the power
	cbs.Sort()
	for i, cb := range cbs {
		if i >= loadParams(store).MaxVals {
			cb.VotingPower = 0
		}
	}
	saveBonds(store, cbs)
	return
}

// CleanupEmpty - removes all validators which have no bonded atoms left
func (cbs Candidates) CleanupEmpty(store state.SimpleDB) {
	for i, cb := range cbs {
		if cb.Tickets == 0 {
			var err error
			cbs, err = cbs.Remove(i)
			if err != nil {
				cmn.PanicSanity(resBadRemoveValidator.Error())
			}
		}
	}
	saveBonds(store, cbs)
}

// GetValidators - get the most recent updated validator set from the
// Candidates. These bonds are already sorted by VotingPower from
// the UpdateVotingPower function which is the only function which
// is to modify the VotingPower
func (cbs Candidates) GetValidators(store state.SimpleDB) []*abci.Validator {
	maxVals := loadParams(store).MaxVals
	validators := make([]*abci.Validator, cmn.MinInt(len(cbs), maxVals))
	for i, cb := range cbs {
		if cb.VotingPower == 0 { //exit as soon as the first Voting power set to zero is found
			break
		}
		if i >= maxVals {
			return validators
		}
		validators[i] = cb.ABCIValidator()
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

// GetByOwner - get a Candidate for a specific sender from the Candidates
func (cbs Candidates) GetByOwner(owner sdk.Actor) (int, *Candidate) {
	for i, cb := range cbs {
		if cb.Owner.Equals(owner) {
			return i, cb
		}
	}
	return 0, nil
}

// GetByPubKey - get a Candidate for a specific validator from the Candidates
func (cbs Candidates) GetByPubKey(pubkey crypto.PubKey) (int, *Candidate) {
	for i, cb := range cbs {
		if cb.PubKey.Equals(pubkey) {
			return i, cb
		}
	}
	return 0, nil
}

// Add - adds a Candidate
func (cbs Candidates) Add(bond *Candidate) Candidates {
	return append(cbs, bond)
}

// Remove - remove validator from the validator list
func (cbs Candidates) Remove(i int) (Candidates, error) {
	switch {
	case i < 0:
		return cbs, fmt.Errorf("Cannot remove a negative element")
	case i >= len(cbs):
		return cbs, fmt.Errorf("Element is out of upper bound")
	default:
		return append(cbs[:i], cbs[i+1:]...), nil
	}
}

//--------------------------------------------------------------------------------

// DelegatorBond represents some bond tokens held by an account.
// It is owned by one delegator, and is associated with the voting power of one pubKey.
type DelegatorBond struct {
	PubKey  crypto.PubKey
	Tickets uint64
}

// DelegatorBonds - all delegator bonds existing with multiple delegatees
type DelegatorBonds []*DelegatorBond

// Get - get a DelegateeBond for a specific validator from the DelegateeBonds
func (b DelegatorBonds) Get(pubKey crypto.PubKey) (int, *DelegatorBond) {
	for i, bv := range b {
		if bv.PubKey.Equal(pubKey) &&
			bv.PubKey.ChainID == pubKey.ChainID &&
			bv.PubKey.App == pubKey.App {
			return i, bv
		}
	}
	return 0, nil
}

// Remove - remove pubKey from the pubKey list
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
