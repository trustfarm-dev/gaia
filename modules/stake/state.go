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
	CandidatesPubKeysKey = []byte{0x01} // key for all candidates' pubkeys
	ParamKey             = []byte{0x02} // key for global parameters relating to staking

	// Key prefixes
	CandidateKeyPrefix           = []byte{0x03} // prefix for each key to a candidate
	CandidateDelegatorsKeyPrefix = []byte{0x04} // prefix for each key for all the delegators to a single candidate
	DelegatorBondKeyPrefix       = []byte{0x05} // prefix for each key to a delegator's bond
	DelegatorCandidatesKeyPrefix = []byte{0x06} // XXX uses a prefix now key for the set of all bonds for a delegator
)

// GetCandidateKey - get the key for the candidate with pubKey
func GetCandidateKey(pubKey crypto.PubKey) []byte {
	return append(CandidateKeyPrefix, pubKey.Bytes()...)
}

func getCandidateDelegatorsKey(pubKey crypto.PubKey) []byte {
	return append(CandidateDelegatorsKeyPrefix, pubKey.Bytes()...)
}

func getDelegatorBondKey(delegator sdk.Actor, candidate crypto.PubKey) []byte {
	bondBytes := append(wire.BinaryBytes(&delegator), candidate.Bytes()...)
	return append(DelegatorBondKeyPrefix, bondBytes...)
}

func getDelegatorCandidatesKey(delegator sdk.Actor) []byte {
	return append(DelegatorCandidatesKeyPrefix, wire.BinaryBytes(&delegator)...)
}

/////////////////////////////////////////////////////////////////////////////////

// Get the active list of all the candidate pubKeys and owners
func loadCandidatesPubKeys(store state.SimpleDB) (pubKeys []crypto.PubKey) {
	bytes := store.Get(CandidatesPubKeysKey)
	if bytes == nil {
		return
	}
	err := wire.ReadBinaryBytes(bytes, &pubKeys)
	if err != nil {
		panic(err)
	}
	return
}
func saveCandidatesPubKeys(store state.SimpleDB, pubKeys []crypto.PubKey) {
	b := wire.BinaryBytes(pubKeys)
	store.Set(CandidatesPubKeysKey, b)
}

// LoadCandidate - loads the pubKey bond set
// TODO ultimately this function should be made unexported... being used right now
// for patchwork of tick functionality therefor much easier if exported until
// the new SDK is created
func LoadCandidate(store state.SimpleDB, pubKey crypto.PubKey) (candidate *Candidate) {
	b := store.Get(CandidateKeyPrefix)
	if b == nil {
		return
	}
	err := wire.ReadBinaryBytes(b, candidate)
	if err != nil {
		panic(err) // This error should never occure big problem if does
	}
	return
}

func saveCandidate(store state.SimpleDB, candidate *Candidate) {
	b := wire.BinaryBytes(*candidate)
	store.Set(CandidateKeyPrefix, b)

	// TODO to be replaced with iteration in the multistore?
	pks := loadCandidatesPubKeys(store)
	saveCandidatesPubKeys(store, append(pks, candidate.PubKey))
}

func removeCandidate(store state.SimpleDB, pubKey crypto.PubKey) {
	store.Remove(GetCandidateKey(pubKey))

	// TODO to be replaced with iteration in the multistore?
	pks := loadCandidatesPubKeys(store)
	for i := range pks {
		if pks[i].Equals(pubKey) {
			saveCandidatesPubKeys(store,
				append(pks[:i], pks[i+1:]...))
			break
		}
	}
}

/////////////////////////////////////////////////////////////////////////////////

// Get the active list of all delegators to a single candidate
func loadCandidateDelegators(store state.SimpleDB,
	pubKey crypto.PubKey) (delegators []sdk.Actor) {

	bytes := store.Get(getCandidateDelegatorsKey(pubKey))
	if bytes == nil {
		return
	}
	err := wire.ReadBinaryBytes(bytes, &delegators)
	if err != nil {
		panic(err)
	}
	return
}
func saveCandidateDelegators(store state.SimpleDB,
	pubKey crypto.PubKey, delegators []sdk.Actor) {

	b := wire.BinaryBytes(delegators)
	store.Set(getCandidateDelegatorsKey(pubKey), b)
}
func loadDelegatorCandidates(store state.SimpleDB,
	delegator sdk.Actor) (candidates []crypto.PubKey) {

	bytes := store.Get(getDelegatorCandidatesKey(delegator))
	if bytes == nil {
		return
	}
	err := wire.ReadBinaryBytes(bytes, &candidates)
	if err != nil {
		panic(err)
	}
	return
}
func saveDelegatorCandidates(store state.SimpleDB,
	delegator sdk.Actor, candidates []crypto.PubKey) {

	b := wire.BinaryBytes(candidates)
	store.Set(getDelegatorCandidatesKey(delegator), b)
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
	if loadDelegatorBond(store, delegator, bond.PubKey) != nil {
		dcs := loadDelegatorCandidates(store, delegator)
		saveDelegatorCandidates(store, delegator, append(dcs, bond.PubKey))

		cds := loadCandidateDelegators(store, bond.PubKey)
		saveCandidateDelegators(store, bond.PubKey, append(cds, delegator))
	}

	b := wire.BinaryBytes(bond)
	store.Set(getDelegatorBondKey(delegator, bond.PubKey), b)
}

func removeDelegatorBond(store state.SimpleDB, delegator sdk.Actor, candidate crypto.PubKey) {
	store.Remove(getDelegatorBondKey(delegator, candidate))

	// TODO to be replaced with iteration in the multistore
	dcs := loadDelegatorCandidates(store, delegator)
	for i := range dcs {
		if dcs[i].Equals(candidate) {
			saveDelegatorCandidates(store, delegator,
				append(dcs[:i], dcs[i+1:]...))
			break
		}
	}

	cds := loadCandidateDelegators(store, candidate)
	for i := range cds {
		if cds[i].Equals(delegator) {
			saveCandidateDelegators(store, candidate,
				append(cds[:i], cds[i+1:]...))
			break
		}
	}
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
