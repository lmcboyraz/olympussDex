package state

import (
	"testing"

	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

func TestGenesisFixtureHasSpecifiedBalancesAndLiabilities(t *testing.T) {
	genesis := GenesisFixture()
	for index, account := range genesis.Accounts {
		if account.BaseBalance != 100 || account.QuoteBalance != 20_000 {
			t.Fatalf("account %d = %+v", index, account)
		}
	}
	if genesis.AMM.BaseReserve != 1_000 || genesis.AMM.QuoteReserve != 100_000 {
		t.Fatalf("AMM = %+v", genesis.AMM)
	}
	if genesis.Metadata.ProcessedBatchCount != 0 ||
		genesis.Metadata.ProcessedMessageCount != 0 {
		t.Fatalf("metadata = %+v", genesis.Metadata)
	}

	liabilities, err := genesis.Liabilities()
	if err != nil {
		t.Fatalf("calculate liabilities: %v", err)
	}
	if liabilities.Base != 1_800 || liabilities.Quote != 260_000 {
		t.Fatalf("liabilities = %+v", liabilities)
	}
}

func TestEveryGenesisLeafHasAValidMerkleProof(t *testing.T) {
	leaves, err := GenesisFixture().Leaves()
	if err != nil {
		t.Fatalf("build genesis leaves: %v", err)
	}
	tree, err := BuildTree(leaves[:])
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}
	root := tree.Root()

	for index, leaf := range leaves {
		proof, err := tree.Proof(index)
		if err != nil {
			t.Fatalf("proof for leaf %d: %v", index, err)
		}
		if !VerifyProof(leaf, proof, root) {
			t.Fatalf("proof for leaf %d did not verify", index)
		}
	}
}

func TestMerkleProofRejectsWrongLeafIndexAndSibling(t *testing.T) {
	leaves, err := GenesisFixture().Leaves()
	if err != nil {
		t.Fatalf("build genesis leaves: %v", err)
	}
	tree, err := BuildTree(leaves[:])
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}
	root := tree.Root()
	proof, err := tree.Proof(3)
	if err != nil {
		t.Fatalf("proof: %v", err)
	}

	var one fr.Element
	one.SetOne()
	wrongLeaf := leaves[3]
	wrongLeaf.Add(&wrongLeaf, &one)
	if VerifyProof(wrongLeaf, proof, root) {
		t.Fatal("wrong leaf unexpectedly verified")
	}

	wrongIndex := proof
	wrongIndex.Index = 2
	if VerifyProof(leaves[3], wrongIndex, root) {
		t.Fatal("wrong index unexpectedly verified")
	}

	wrongSibling := proof
	wrongSibling.Siblings[0].Add(&wrongSibling.Siblings[0], &one)
	if VerifyProof(leaves[3], wrongSibling, root) {
		t.Fatal("wrong sibling unexpectedly verified")
	}
}

func TestUpdateLeafChangesRootAndInvalidatesOldProof(t *testing.T) {
	leaves, err := GenesisFixture().Leaves()
	if err != nil {
		t.Fatalf("build genesis leaves: %v", err)
	}
	tree, err := BuildTree(leaves[:])
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}
	oldRoot := tree.Root()
	oldProof, err := tree.Proof(0)
	if err != nil {
		t.Fatalf("old proof: %v", err)
	}
	updatedLeaf, err := AccountLeaf(0, 101, 20_000)
	if err != nil {
		t.Fatalf("updated account leaf: %v", err)
	}

	newRoot, err := tree.UpdateLeaf(0, updatedLeaf)
	if err != nil {
		t.Fatalf("update leaf: %v", err)
	}
	if newRoot.Equal(&oldRoot) {
		t.Fatal("root did not change")
	}
	if VerifyProof(leaves[0], oldProof, newRoot) {
		t.Fatal("old proof unexpectedly verified against new root")
	}
	newProof, err := tree.Proof(0)
	if err != nil {
		t.Fatalf("new proof: %v", err)
	}
	if !VerifyProof(updatedLeaf, newProof, newRoot) {
		t.Fatal("updated leaf proof did not verify")
	}
}

func TestCanonicalEmptyLeavesAreBoundToTheirIndexes(t *testing.T) {
	seen := make(map[string]int)
	for index := protocol.FirstEmptyLeafIndex; index < protocol.LeafCount; index++ {
		leaf, err := EmptyLeaf(index)
		if err != nil {
			t.Fatalf("empty leaf %d: %v", index, err)
		}
		key := leaf.String()
		if previous, exists := seen[key]; exists {
			t.Fatalf("empty leaves %d and %d collide", previous, index)
		}
		seen[key] = index
	}
}

func TestGenesisRootIsDeterministic(t *testing.T) {
	first, err := GenesisRoot()
	if err != nil {
		t.Fatalf("first genesis root: %v", err)
	}
	second, err := GenesisRoot()
	if err != nil {
		t.Fatalf("second genesis root: %v", err)
	}
	if !first.Equal(&second) {
		t.Fatalf("genesis roots differ: %s != %s", first.String(), second.String())
	}
}

func TestMerkleHelpersRejectInvalidIndexesAndLeafCounts(t *testing.T) {
	if _, err := BuildTree(make([]fr.Element, protocol.LeafCount-1)); err == nil {
		t.Fatal("short leaf set unexpectedly accepted")
	}

	leaves, err := GenesisFixture().Leaves()
	if err != nil {
		t.Fatalf("build genesis leaves: %v", err)
	}
	tree, err := BuildTree(leaves[:])
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}
	if _, err := tree.Proof(-1); err == nil {
		t.Fatal("negative proof index unexpectedly accepted")
	}
	if _, err := tree.Proof(protocol.LeafCount); err == nil {
		t.Fatal("high proof index unexpectedly accepted")
	}
	if _, err := tree.UpdateLeaf(protocol.LeafCount, fr.Element{}); err == nil {
		t.Fatal("high update index unexpectedly accepted")
	}
}
