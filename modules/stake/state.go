package stake

import (
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/state"
	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/go-wire"
)

// nolint
var (
	// Keys for store prefixes
	CandidateKeyPrefix           = []byte{0x00} // prefix for each key to a candidate
	CandidateDelegatorsKeyPrefix = []byte{0x01} // XXX prefix for each key for all the delegators to a single candidate
	CandidatesPubKeysKey         = []byte{0x02} // key for all candidates' pubkeys
	DelegatorBondKeyPrefix       = []byte{0x03} // prefix for each key to a delegator's bond
	DelegatorCandidatesKeyPrefix = []byte{0x04} // XXX uses a prefix now key for the set of all bonds for a delegator
	ParamKey                     = []byte{0x05} // key for global parameters relating to staking
)

func getDelegatorBondKey(delegator sdk.Actor, candidate crypto.PubKey) []byte {
	bondBytes := append(wire.BinaryBytes(&delegator), candidate.Bytes()...)
	return append(DelegatorBondKeyPrefix, bondBytes...)
}

func getCandidateKey(pubkey crypto.PubKey) []byte {
	return append(DelegatorBondKeyPrefix, candidate.Bytes()...)
}

/////////////////////////////////////////////////////////////////////////////////

// Get the active list of all the candidate pubKeys and owners
func loadCandidatesPubKeys(store state.SimpleDB) (pubKeys map[crypto.PubKey]struct{}) {
	bytes := store.Get(CandidateListKey)
	if bytes == nil {
		return
	}
	err := wire.ReadBinaryBytes(bytes, &pubKeys)
	if err != nil {
		panic(err)
	}
	return
}

// Get the active list of all the candidate pubKeys and owners
func saveCandidatesPubKeys(store state.SimpleDB, pubKeys map[crypto.PubKey]struct{}) {
	b := wire.BinaryBytes(pubKeys)
	store.Set(CandidateListKey, b)
}

// LoadCandidate - loads the pubKey bond set
// TODO ultimately this function should be made unexported... being used right now
// for patchwork of tick functionality therefor much easier if exported until
// the new SDK is created
func LoadCandidate(store state.SimpleDB, pubKey crypto.PubKey) (candidate *Candidate) {
	b := store.Get(CandidateListKey)
	if b == nil {
		return
	}
	err := wire.ReadBinaryBytes(b, candidates)
	if err != nil {
		panic(err) // This error should never occure big problem if does
	}
	return
}

func saveCandidate(store state.SimpleDB, candidate *Candidate) {
	b := wire.BinaryBytes(*candidate)
	store.Set(CandidateKeyPrefix, b)

	// TODO to be replaced with iteration in the multistore?
	pks := loadCandidatePubKeys(store)
	pks[candidate.PubKey] = struct{}{} //set the key in the map of all candidates
	saveCandidatePubKeys(store, pks)
}

func removeCandidate(store state.SimpleDB, pubKey crypto.PubKey) {
	store.Remove(getCandidateKey(delegator, pubKey))

	// TODO to be replaced with iteration in the multistore?
	pks := loadCandidatesPubKeys(store)
	delete(pks, pubKey)
	saveCandidatePubKeys(store, pks)
}

/////////////////////////////////////////////////////////////////////////////////

// Get the active list of all delegators to a single candidate
func loadCandidateDelegators(store state.SimpleDB,
	pubKey crypto.PubKey) (delegators map[sdk.Actor]struct{}) {

	bytes := store.Get(CandidateDelegatorsKey)
	if bytes == nil {
		return
	}
	err := wire.ReadBinaryBytes(bytes, &delegators)
	if err != nil {
		panic(err)
	}
	return
}
func saveCandidateDelegators(store state.SimpleDB, pubKeys map[crypto.PubKey]struct{}) {
	b := wire.BinaryBytes(pubKeys)
	store.Set(CandidateListKey, b)
}
func loadDelegatorCandidates(store state.SimpleDB,
	delegator sdk.Actor) (candidates map[crypto.PubKey]struct{}) {

	bytes := store.Get(DelegatorBondListKey)
	if bytes == nil {
		return
	}
	err := wire.ReadBinaryBytes(bytes, &candidates)
	if err != nil {
		panic(err)
	}
	return
}

func loadDelegatorBond(store state.SimpleDB,
	delegator sdk.Actor, candidate crypto.PubKey) (bond *DelegatorBond) {

	delegatorBytes := store.Get(getDelegatorBondKey(delegator, candidate))
	if delegatorBytes == nil {
		return
	}

	err := wire.ReadBinaryBytes(delegatorBytes, bond)
	if err != nil {
		panic(err)
	}
	return
}

func saveDelegatorBond(store state.SimpleDB, delegator sdk.Actor, bond DelegatorBond) {

	//if a new record also add to the list of all delegated candidates for this delegator
	if len(store.Get(loadDelegatorBond(store, delegator, bond.PubKey))) > 0 {
		dcs := loadDelegatorCandidates(store, delegator)
		store.Set(DelegatorCandidatesKey, append(dcs, bond.PubKey))

		cds := store.loadCandidateDelegators(store, bond.PubKey)
		store.Set(DelegatorCandidatesKey, append(cds, delegator))
	}

	b := wire.BinaryBytes(bond)
	store.Set(getDelegatorBondsKey(delegator, bond.PubKey), b)
}

func removeDelegatorBond(store state.SimpleDB, delegator sdk.Actor, candidate crypto.PubKey) {
	store.Remove(getDelegatorBondKey(delegator, candidate))

	// TODO to be replaced with iteration in the multistore
	dcs := loadDelegatorCandidates(store, delegator)
	store.Set(DelegatorCandidatesKey, append(dcs, bond.PubKey))

	cds := loadCandidateDelegators(store, bond.PubKey)
	store.set(getDelegatorBondKey(delegator, candidate))
}

/////////////////////////////////////////////////////////////////////////////////

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
