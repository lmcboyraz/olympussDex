package state

import (
	"fmt"

	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

type Tree struct {
	levels [protocol.TreeDepth + 1][]fr.Element
}

type MerkleProof struct {
	Index    int
	Siblings [protocol.TreeDepth]fr.Element
}

func BuildTree(leaves []fr.Element) (*Tree, error) {
	if len(leaves) != protocol.LeafCount {
		return nil, fmt.Errorf("leaf count = %d, want %d", len(leaves), protocol.LeafCount)
	}

	tree := &Tree{}
	tree.levels[0] = append([]fr.Element(nil), leaves...)
	for level := 0; level < protocol.TreeDepth; level++ {
		current := tree.levels[level]
		next := make([]fr.Element, len(current)/2)
		for index := range next {
			parent, err := internalNode(level, current[index*2], current[index*2+1])
			if err != nil {
				return nil, err
			}
			next[index] = parent
		}
		tree.levels[level+1] = next
	}
	return tree, nil
}

func (tree *Tree) Root() fr.Element {
	if tree == nil || len(tree.levels[protocol.TreeDepth]) != 1 {
		return fr.Element{}
	}
	return tree.levels[protocol.TreeDepth][0]
}

func (tree *Tree) Proof(index int) (MerkleProof, error) {
	if tree == nil {
		return MerkleProof{}, fmt.Errorf("tree is nil")
	}
	if index < 0 || index >= protocol.LeafCount {
		return MerkleProof{}, fmt.Errorf("proof index %d is outside [0,%d]", index, protocol.LeafCount-1)
	}

	proof := MerkleProof{Index: index}
	nodeIndex := index
	for level := 0; level < protocol.TreeDepth; level++ {
		proof.Siblings[level] = tree.levels[level][nodeIndex^1]
		nodeIndex /= 2
	}
	return proof, nil
}

func VerifyProof(leaf fr.Element, proof MerkleProof, root fr.Element) bool {
	if proof.Index < 0 || proof.Index >= protocol.LeafCount {
		return false
	}

	current := leaf
	nodeIndex := proof.Index
	for level := 0; level < protocol.TreeDepth; level++ {
		var left, right fr.Element
		if nodeIndex&1 == 0 {
			left, right = current, proof.Siblings[level]
		} else {
			left, right = proof.Siblings[level], current
		}
		parent, err := internalNode(level, left, right)
		if err != nil {
			return false
		}
		current = parent
		nodeIndex /= 2
	}
	return current.Equal(&root)
}

func (tree *Tree) UpdateLeaf(index int, leaf fr.Element) (fr.Element, error) {
	if tree == nil {
		return fr.Element{}, fmt.Errorf("tree is nil")
	}
	if index < 0 || index >= protocol.LeafCount {
		return fr.Element{}, fmt.Errorf("update index %d is outside [0,%d]", index, protocol.LeafCount-1)
	}

	tree.levels[0][index] = leaf
	nodeIndex := index
	for level := 0; level < protocol.TreeDepth; level++ {
		parentIndex := nodeIndex / 2
		parent, err := internalNode(
			level,
			tree.levels[level][parentIndex*2],
			tree.levels[level][parentIndex*2+1],
		)
		if err != nil {
			return fr.Element{}, err
		}
		tree.levels[level+1][parentIndex] = parent
		nodeIndex = parentIndex
	}
	return tree.Root(), nil
}
