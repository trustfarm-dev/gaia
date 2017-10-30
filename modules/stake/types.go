package stake

import (
	"bytes"
	"sort"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/state"
	abci "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
	wire "github.com/tendermint/go-wire"
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

// Candidate defines the total amount of bond shares and their exchange rate to
// coins, associated with a single validator. Accumulation of interest is modelled
// as an in increase in the exchange rate, and slashing as a decrease.
// When coins are delegated to this validator, the validator is credited
// with a DelegatorBond whose number of bond shares is based on the amount of coins
// delegated divided by the current exchange rate. Voting power can be calculated as
// total bonds multiplied by exchange rate.
type Candidate struct {
	PubKey      crypto.PubKey // Pubkey of validator
	Owner       sdk.Actor     // Sender of BondTx - UnbondTx returns here
	Shares      uint64        // Total number of bond shares for the validator, equivalent to coins held in bond account
	VotingPower uint64        // Voting power if pubKey is a considered a validator
	Delegators  []sdk.Actor   // List of all delegators to this Candidate
}

// NewCandidate - returns a new empty validator bond object
func NewCandidate(owner sdk.Actor, pubKey crypto.PubKey) *Candidate {
	return &Candidate{
		Owner:       owner,
		PubKey:      pubKey,
		Shares:      0,
		VotingPower: 0,
		Delegators:  []sdk.Actor{}, // start empty
	}
}

// ABCIValidator - Get the validator from a bond value
func (c Candidate) ABCIValidator() *abci.Validator {
	return &abci.Validator{
		PubKey: wire.BinaryBytes(c.PubKey),
		Power:  c.VotingPower,
	}
}

// HoldAccount - Get the hold account for the Candidate
func (c Candidate) HoldAccount() sdk.Actor {
	return getHoldAccount(c.Owner)
}

// getHoldAccount - the account where bonded atoms are held only accessed by protocol
func getHoldAccount(owner sdk.Actor) sdk.Actor {
	holdAddr := append([]byte{0x00}, owner.Address[1:]...) //shift and prepend a zero
	return sdk.NewActor(stakingModuleName, holdAddr)
}

//--------------------------------------------------------------------------------

// TODO replace with sorted multistore functionality

// Candidates - list of Candidates
type Candidates []*Candidate

var _ sort.Interface = Candidates{} //enforce the sort interface at compile time

// LoadCandidates - TODO replace with  multistore
func LoadCandidates(store state.SimpleDB) (candidates Candidates) {
	pks := loadCandidatesPubKeys(store)
	for _, pk := range pks {
		candidates = append(candidates, LoadCandidate(store, pk))
	}
	return
}

// nolint - sort interface functions
func (cs Candidates) Len() int      { return len(cs) }
func (cs Candidates) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }
func (cs Candidates) Less(i, j int) bool {
	vp1, vp2 := cs[i].VotingPower, cs[j].VotingPower
	d1, d2 := cs[i].Owner, cs[j].Owner
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
func (cs Candidates) Sort() {
	sort.Sort(cs)
}

//--------------------------------------------------------------------------------

// UpdateVotingPower - voting power based on bond shares and exchange rate
func (cs Candidates) UpdateVotingPower(store state.SimpleDB) {
	for _, c := range cs {
		c.VotingPower = c.Shares
	}

	// Now sort and truncate the power
	cs.Sort()
	for i, c := range cs {
		if i >= loadParams(store).MaxVals {
			c.VotingPower = 0
		}
		saveCandidate(store, c)
	}
	return
}

// GetValidators - get the most recent updated validator set from the
// Candidates. These bonds are already sorted by VotingPower from
// the UpdateVotingPower function which is the only function which
// is to modify the VotingPower
func (cs Candidates) GetValidators(store state.SimpleDB) Candidates {
	maxVals := loadParams(store).MaxVals
	validators := make(Candidates, cmn.MinInt(len(cs), maxVals))
	for i, c := range cs {
		if c.VotingPower == 0 { //exit as soon as the first Voting power set to zero is found
			break
		}
		if i >= maxVals {
			return validators
		}
		validators[i] = c
	}
	return validators
}

// ValidatorsDiff - get the difference in the validator set from the input validator set
func ValidatorsDiff(previous, current Candidates, store state.SimpleDB) (diff []*abci.Validator) {

	//TODO do something more efficient possibly by sorting first

	//calculate any differences from the previous to the new validator set
	// first loop through the previous validator set, and then catch any
	// missed records in the new validator set
	diff = make([]*abci.Validator, 0, loadParams(store).MaxVals)

	for _, prevVal := range previous {
		abciVal := prevVal.ABCIValidator()
		if prevVal == nil {
			continue
		}
		found := false
		candidate := LoadCandidate(store, prevVal.PubKey)
		if candidate != nil {
			found = true
			if candidate.VotingPower != prevVal.VotingPower {
				diff = append(diff, &abci.Validator{abciVal.PubKey, candidate.VotingPower})
			}
		}
		if !found {
			diff = append(diff, &abci.Validator{abciVal.PubKey, 0})
		}
	}
	for _, curVal := range current {
		if curVal == nil {
			continue
		}
		candidate := LoadCandidate(store, curVal.PubKey)
		if candidate == nil {
			diff = append(diff, curVal.ABCIValidator())
		}
	}
	return
}

//--------------------------------------------------------------------------------

// DelegatorBond represents some bond tokens held by an account.
// It is owned by one delegator, and is associated with the voting power of one pubKey.
type DelegatorBond struct {
	PubKey crypto.PubKey
	Shares uint64
}

//--------------------------------------------------------------------------------

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
