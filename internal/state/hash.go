package state

import (
	"fmt"

	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon2"
)

func AccountLeaf(
	accountIndex int,
	baseBalance uint64,
	quoteBalance uint64,
) (fr.Element, error) {
	if accountIndex < 0 || accountIndex >= protocol.AccountLeafCount {
		return fr.Element{}, fmt.Errorf(
			"account index %d is outside [0,%d]",
			accountIndex,
			protocol.AccountLeafCount-1,
		)
	}
	domains := Domains()
	return hashElements(
		domains.AccountLeaf,
		fr.NewElement(uint64(accountIndex)),
		fr.NewElement(baseBalance),
		fr.NewElement(quoteBalance),
	)
}

func AMMLeaf(baseReserve uint64, quoteReserve uint64) (fr.Element, error) {
	domains := Domains()
	return hashElements(
		domains.AMMLeaf,
		fr.NewElement(protocol.AMMLeafIndex),
		fr.NewElement(baseReserve),
		fr.NewElement(quoteReserve),
	)
}

func MetadataLeaf(
	processedBatchCount uint64,
	processedMessageCount uint64,
) (fr.Element, error) {
	domains := Domains()
	return hashElements(
		domains.MetadataLeaf,
		fr.NewElement(protocol.MetadataLeafIndex),
		fr.NewElement(processedBatchCount),
		fr.NewElement(processedMessageCount),
	)
}

func EmptyLeaf(leafIndex int) (fr.Element, error) {
	if leafIndex < protocol.FirstEmptyLeafIndex || leafIndex >= protocol.LeafCount {
		return fr.Element{}, fmt.Errorf(
			"empty leaf index %d is outside [%d,%d]",
			leafIndex,
			protocol.FirstEmptyLeafIndex,
			protocol.LeafCount-1,
		)
	}
	domains := Domains()
	return hashElements(
		domains.EmptyLeaf,
		fr.NewElement(uint64(leafIndex)),
	)
}

func internalNode(
	level int,
	left fr.Element,
	right fr.Element,
) (fr.Element, error) {
	if level < 0 || level >= protocol.TreeDepth {
		return fr.Element{}, fmt.Errorf("internal-node level %d is outside [0,%d]", level, protocol.TreeDepth-1)
	}
	domains := Domains()
	return hashElements(
		domains.InternalNode,
		fr.NewElement(uint64(level)),
		left,
		right,
	)
}

func hashElements(elements ...fr.Element) (fr.Element, error) {
	hasher := poseidon2.NewMerkleDamgardHasher()
	for index := range elements {
		encoded := elements[index].Bytes()
		written, err := hasher.Write(encoded[:])
		if err != nil {
			return fr.Element{}, fmt.Errorf("Poseidon2 element %d: %w", index, err)
		}
		if written != fr.Bytes {
			return fr.Element{}, fmt.Errorf(
				"Poseidon2 element %d wrote %d bytes, want %d",
				index,
				written,
				fr.Bytes,
			)
		}
	}

	var result fr.Element
	if err := result.SetBytesCanonical(hasher.Sum(nil)); err != nil {
		return fr.Element{}, fmt.Errorf("decode Poseidon2 digest: %w", err)
	}
	return result, nil
}
