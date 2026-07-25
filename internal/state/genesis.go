package state

import (
	"fmt"
	"math"

	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

type Account struct {
	BaseBalance  uint64
	QuoteBalance uint64
}

type AMM struct {
	BaseReserve  uint64
	QuoteReserve uint64
}

type Metadata struct {
	ProcessedBatchCount   uint64
	ProcessedMessageCount uint64
}

type State struct {
	Accounts [protocol.AccountLeafCount]Account
	AMM      AMM
	Metadata Metadata
}

type Liabilities struct {
	Base  uint64
	Quote uint64
}

func GenesisFixture() State {
	var state State
	for index := range state.Accounts {
		state.Accounts[index] = Account{
			BaseBalance:  100,
			QuoteBalance: 20_000,
		}
	}
	state.AMM = AMM{
		BaseReserve:  1_000,
		QuoteReserve: 100_000,
	}
	return state
}

func (state State) Liabilities() (Liabilities, error) {
	liabilities := Liabilities{}
	for index, account := range state.Accounts {
		var err error
		liabilities.Base, err = addUint64(liabilities.Base, account.BaseBalance)
		if err != nil {
			return Liabilities{}, fmt.Errorf("account %d base liability: %w", index, err)
		}
		liabilities.Quote, err = addUint64(liabilities.Quote, account.QuoteBalance)
		if err != nil {
			return Liabilities{}, fmt.Errorf("account %d quote liability: %w", index, err)
		}
	}
	var err error
	liabilities.Base, err = addUint64(liabilities.Base, state.AMM.BaseReserve)
	if err != nil {
		return Liabilities{}, fmt.Errorf("AMM base liability: %w", err)
	}
	liabilities.Quote, err = addUint64(liabilities.Quote, state.AMM.QuoteReserve)
	if err != nil {
		return Liabilities{}, fmt.Errorf("AMM quote liability: %w", err)
	}
	return liabilities, nil
}

func (state State) Leaves() ([protocol.LeafCount]fr.Element, error) {
	var leaves [protocol.LeafCount]fr.Element
	for index, account := range state.Accounts {
		leaf, err := AccountLeaf(index, account.BaseBalance, account.QuoteBalance)
		if err != nil {
			return leaves, err
		}
		leaves[index] = leaf
	}

	ammLeaf, err := AMMLeaf(state.AMM.BaseReserve, state.AMM.QuoteReserve)
	if err != nil {
		return leaves, err
	}
	leaves[protocol.AMMLeafIndex] = ammLeaf

	metadataLeaf, err := MetadataLeaf(
		state.Metadata.ProcessedBatchCount,
		state.Metadata.ProcessedMessageCount,
	)
	if err != nil {
		return leaves, err
	}
	leaves[protocol.MetadataLeafIndex] = metadataLeaf

	for index := protocol.FirstEmptyLeafIndex; index < protocol.LeafCount; index++ {
		emptyLeaf, err := EmptyLeaf(index)
		if err != nil {
			return leaves, err
		}
		leaves[index] = emptyLeaf
	}
	return leaves, nil
}

func GenesisRoot() (fr.Element, error) {
	leaves, err := GenesisFixture().Leaves()
	if err != nil {
		return fr.Element{}, err
	}
	tree, err := BuildTree(leaves[:])
	if err != nil {
		return fr.Element{}, err
	}
	return tree.Root(), nil
}

func addUint64(left uint64, right uint64) (uint64, error) {
	if left > math.MaxUint64-right {
		return 0, fmt.Errorf("uint64 overflow")
	}
	return left + right, nil
}
