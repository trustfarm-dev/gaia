package stake

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	crypto "github.com/tendermint/go-crypto"
)

// Tx
//--------------------------------------------------------------------------------

// register the tx type with its validation logic
// make sure to use the name of the handler as the prefix in the tx type,
// so it gets routed properly
const (
	ByteTxBond   = 0x55
	ByteTxUnbond = 0x56
	TypeTxBond   = stakingModuleName + "/bond"
	TypeTxUnbond = stakingModuleName + "/unbond"
)

func init() {
	sdk.TxMapper.RegisterImplementation(TxBond{}, TypeTxBond, ByteTxBond)
	sdk.TxMapper.RegisterImplementation(TxUnbond{}, TypeTxUnbond, ByteTxUnbond)
}

//Verify interface at compile time
var _, _ sdk.TxInner = &TxBond{}, &TxUnbond{}

// BondUpdate - struct for bonding or unbonding transactions
type BondUpdate struct {
	PubKey crypto.PubKey `json:"pubKey"`
	Bond      coin.Coin     `json:"amount"`
}

// Wrap - Wrap a Tx as a Basecoin Tx
func (tx BondUpdate) Wrap() sdk.Tx {
	return sdk.Tx{tx}
}

// ValidateBasic - Check for non-empty actor, and valid coins
func (tx BondUpdate) ValidateBasic() error {
	if len(tx.PubKey.Bytes()) == 0 { // TODO will an empty validator actually have len 0?
		return errValidatorEmpty
	}

	coins := coin.Coins{tx.Amount}
	if !coins.IsValid() {
		return coin.ErrInvalidCoins()
	}
	if !coins.IsPositive() {
		return fmt.Errorf("Amount must be > 0")
	}
	if coins[0].Denom != bondDenom {
		return fmt.Errorf("Invalid coin denomination")
	}
	return nil
}

// TxBond - struct for bonding transactions
type TxBond struct{ BondUpdate }

// NewTxBond - new TxBond
func NewTxBond(amount coin.Coin, pubKey crypto.PubKey) sdk.Tx {
	return TxBond{BondUpdate{
		Amount: amount,
		PubKey: pubKey,
	}}.Wrap()
}

// TxUnbond - struct for unbonding transactions
type TxUnbond struct{ BondUpdate }

// NewTxUnbond - new TxUnbond
func NewTxUnbond(amount coin.Coin, pubKey crypto.PubKey) sdk.Tx {
	return TxUnbond{BondUpdate{
		Amount: amount,
		PubKey: pubKey,
	}}.Wrap()
}

// TxDeclareCandidacy - struct for unbonding transactions
type TxDeclareCandidacy struct{ BondUpdate }

// NewTxUnbond - new TxDeclareCandidacy
func NewTxUnbond(amount coin.Coin, pubKey crypto.PubKey) sdk.Tx {
	return TxDeclareCandidacy{BondUpdate{
		Amount: amount,
		PubKey: pubKey,
	}}.Wrap()
}
