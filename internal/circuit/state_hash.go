package circuit

import (
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash"
	"github.com/consensys/gnark/std/permutation/poseidon2"
)

const (
	accountLeafDomain  = 0x0101
	ammLeafDomain      = 0x0102
	metadataLeafDomain = 0x0103
	emptyLeafDomain    = 0x0104
	internalNodeDomain = 0x0105
)

// ConstrainStateRoot range-checks the complete private state and recreates the
// exact M1 index/domain/level-bound Poseidon2 tree.
func ConstrainStateRoot(api frontend.API, value StateWitness) (frontend.Variable, error) {
	g := NewGadgets(api)
	var leaves [protocol.LeafCount]frontend.Variable
	for index, account := range value.Accounts {
		g.AssertUnsigned(account.BaseBalance, 64)
		g.AssertUnsigned(account.QuoteBalance, 64)
		leaf, err := poseidonHash(
			api,
			accountLeafDomain,
			index,
			account.BaseBalance,
			account.QuoteBalance,
		)
		if err != nil {
			return nil, err
		}
		leaves[index] = leaf
	}

	g.AssertUnsigned(value.AMM.BaseReserve, 64)
	g.AssertUnsigned(value.AMM.QuoteReserve, 64)
	ammLeaf, err := poseidonHash(
		api,
		ammLeafDomain,
		protocol.AMMLeafIndex,
		value.AMM.BaseReserve,
		value.AMM.QuoteReserve,
	)
	if err != nil {
		return nil, err
	}
	leaves[protocol.AMMLeafIndex] = ammLeaf

	g.AssertUnsigned(value.Metadata.ProcessedBatchCount, 64)
	g.AssertUnsigned(value.Metadata.ProcessedMessageCount, 64)
	metadataLeaf, err := poseidonHash(
		api,
		metadataLeafDomain,
		protocol.MetadataLeafIndex,
		value.Metadata.ProcessedBatchCount,
		value.Metadata.ProcessedMessageCount,
	)
	if err != nil {
		return nil, err
	}
	leaves[protocol.MetadataLeafIndex] = metadataLeaf

	for index := protocol.FirstEmptyLeafIndex; index < protocol.LeafCount; index++ {
		emptyLeaf, err := poseidonHash(api, emptyLeafDomain, index)
		if err != nil {
			return nil, err
		}
		leaves[index] = emptyLeaf
	}

	levelNodes := leaves[:]
	for level := 0; level < protocol.TreeDepth; level++ {
		next := make([]frontend.Variable, len(levelNodes)/2)
		for index := range next {
			parent, err := poseidonHash(
				api,
				internalNodeDomain,
				level,
				levelNodes[index*2],
				levelNodes[index*2+1],
			)
			if err != nil {
				return nil, err
			}
			next[index] = parent
		}
		levelNodes = next
	}
	return levelNodes[0], nil
}

func poseidonHash(
	api frontend.API,
	elements ...frontend.Variable,
) (frontend.Variable, error) {
	permutation, err := poseidon2.NewPoseidon2FromParameters(api, 2, 6, 50)
	if err != nil {
		return nil, err
	}
	hasher := hash.NewMerkleDamgardHasher(api, permutation, 0)
	hasher.Write(elements...)
	return hasher.Sum(), nil
}
