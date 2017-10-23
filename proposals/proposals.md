# Staking Proposals

This document is intended to lay out the implementation order of various
staking features. Each iteration of the staking module should be deployable 
as a test-net. Please contribute to this document and add discussion :)

Overall the Staking module should be rolled out in the following steps

1. Self-Bonding
2. Delegation
3. Unbonding period
4. Rewards, Commission, CryptoEconomics Balancing Model
5. Delegator Rebonding
6. Sub-Delegation

## Self-Bonding

The majority of the processing will happen from the set of all Validator Bonds
which is defined as an array and persisted in the application state.  Each
validator bond is defined as the object below. 
 - Sender: Account which coins are bonded from and unbonded to
 - Pubkey: Validator PubKey
 - BondedTokens: Total number of bond tokens for the validator
 - HoldAccount: Account where the bonded coins are held. Controlled by the app
 - VotingPower: Voting power in consensus

``` golang
type ValidatorBonds []*ValidatorBond

type ValidatorBond struct {
	Sender       sdk.Actor 
	PubKey       []byte    
	BondedTokens uint64    
	HoldAccount  sdk.Actor 
	VotingPower  uint64   
}
```
Note that the Validator PubKey is not necessarily the PubKey of the sender of
the transaction, but is defined by the transaction. This way a separated key
can be used for holding tokens and participating in consensus. 


TxBond is the self-bond SDK transaction for self-bonding tokens to a PubKey
which you designate.  Here the PubKey bytes are the serialized bytes of the
PubKey struct as defined by `tendermint/go-crypto`.

``` golang
type TxBond struct {
	Amount coin.Coin `json:"amount"`
	PubKey crypto.PubKey    `json:"pubkey"`
}
```

Similarity, `TxUnbond` can is used to unbond tokens from the validators bond
account. The key difference is from `TxBond` is that the PubKey does not need
to be included as it has already been associated with the sender account.


``` golang
type TxUnbond struct {
	Amount coin.Coin `json:"amount"`
}
```

## Delegation

The next phase of development includes delegation functionality. All transactions
are still instantaneous in this development level.

The previous struct `ValidatorBond` is now split up into two structs which represent
the candidate account or the bond to a candidate account.

Each validator-candidate bond is defined as the object below. 
 - Sender: Account which coins are bonded from and unbonded to
 - Pubkey: Candidate PubKey
 - Tickets: Total number of tickets provided for the candidate for bonds
 - HoldCoins: Total number of coins held by this validator
 - HoldAccount: Account where the bonded coins are held. Controlled by the app
 - VotingPower: Voting power in consensus

``` golang
type CandidateBond struct {
	Sender       sdk.Actor 
	PubKey       crypto.PubKey
	Tickets      uint64    
    HoldCoins    uint64  
	HoldAccount  sdk.Actor 
	VotingPower  uint64   
}
```

With this model the exchange rate (which is a fraction or decimal) does not
need to be stored and can be calculated as needed as `HoldCoins/Tickets`. 

DelegatorBond represents some bond tokens held by an account. It is owned by
one delegator, and is associated with the voting power of one delegatee.

``` golang
type DelegatorBond struct {
	Candidate  sdk.Actor
	Tickets    uint64
} 
```

`TxBond` and `TxUnbond` can now be used with expanded 
functionality for use from delegators. Now however `TxUnbond` must also include 
the PubKey which the unbonding is to occur from.

``` golang
type TxBond struct {
	Amount coin.Coin        `json:"amount"`
	PubKey crypto.PubKey    `json:"pubkey"`
}
```

Additionally a new type of transaction must be introduced which designates you as a 
Candidate.

``` golang
type TxDeclareCandidacy struct {
	Candidate  sdk.Actor `json:"candidate"`
	SelfBond     coin.Coin `json:"amount"`
	Commission Decimal   `json:"commission"`
}
```

## Unbonding Period

A staking-module state parameter will be introduced which defines the number of
blocks to unbond from a validator. None of the structs should be affected by
this phase. Once complete unbonding as a validator or a Candidate will put your
coins in a queue before being returned.


## Rewards, Commission, CryptoEconomics Balancing Model

Rewards are payed directly to the `HoldAccount` (and reflected in the `HoldCoins`) 
during each reward cycle.

TODO add discussion about the Balancing Model (egg curve)

Each validator will now have the opportunity to charge commission to their
delegators. Included in this is an element self-regulation by validators.
 
``` golang
type CandidateBond struct {
	Sender            sdk.Actor 
	PubKey            crypto.PubKey
	Tickets           uint64    
    HoldCoins         uint64  
	HoldAccount       sdk.Actor 
	VotingPower       uint64   
    CommissionRate    uint64
    CommissionMax     uint64
    CommissionMaxRate uint64
}
```

Four new terms are introduced here:
 - Commission: The current commission rate currently being charged by the validator
 - CommissionRate:  The commission rate charged from validation rewards
 - CommissionMax:  The maximum commission rate which this validator can charge
 - CommissionMaxRate: The maximum change per reward cycle which the validator can change their commission by

These terms will also need to be introduced into `TxDeclareCandidacy`

## Delegator Rebonding

If a delegator is attempting to switch validators, using an unbonding command followed
by a bond command will reduce the amount of validation reward than having just stayed
with the same validator. Because we do not want to disincentive delegators from
switching validators, we may introduce a rebond command which provides equal reward 
to as if you had never unbonded. 

The moment you are “rebonding” the old validator loses your voting power, so the
impact of your bonded atoms on the whatever malicious activity is happening is
technically not ever attributed to you… because of this it’s actually okay to
no longer be slashable for the actions of your old validator from the moment
you unbond… The only reason we are even suggesting that you should be slashable
is to prevent the situation were you are incentivized to be in a constant state
of unbonding. 

Transactions structs of the rebond command may looks like this: 

``` golang
type TxRebond struct {
	PubKey crypto.PubKey  `json:"pubkey"`
	Amount coin.Coin      `json:"amount"`
}
```

Where the PubKey is the PubKey of the new validator to rebond to. 

## Sub-Delegation

Problem: There needs to be a mechanism for delegators to push new Candidates
into the top 100 Validator spots without having to sacrifice bond rewards

Draft idea: Allow for a single layer of sub-delegations… Rather than bonding to
a validator directly you can bond to validator-candidate who is also currently
bonded to a number of running validators. When the validator candidate has
enough tokens to push them into the top 100, they can make a transaction to
rebond to themselves as a full validator. While the candidate is not yet a
validator they do not charge commission themselves, but just pass on the
commission charges from who they are bonded too. From a delegators perspective
I think this is very simple, you’re still just bonding to a validator, you’re
collecting the same rewards they are and trusting their re-delegation until
they accrue enough support to be the real-deal. Computationally also much
better no iteration of the delegator accounts required.

Sub-delegation now changes the order in which operations occur when 
