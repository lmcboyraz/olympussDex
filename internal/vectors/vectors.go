package vectors

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

const GoldenVectorSchemaVersion = 1

type FieldElement struct {
	Decimal string `json:"decimal"`
	Hex     string `json:"hex"`
}

type DomainVectors struct {
	BatchEncodingVersion   int          `json:"batchEncodingVersion"`
	BatchDomainSeparator   string       `json:"batchDomainSeparator"`
	Poseidon2Parameters    string       `json:"poseidon2Parameters"`
	Poseidon2DomainVersion uint64       `json:"poseidon2DomainVersion"`
	AccountLeaf            FieldElement `json:"accountLeaf"`
	AMMLeaf                FieldElement `json:"ammLeaf"`
	MetadataLeaf           FieldElement `json:"metadataLeaf"`
	EmptyLeaf              FieldElement `json:"emptyLeaf"`
	InternalNode           FieldElement `json:"internalNode"`
}

type LeafVector struct {
	Index int          `json:"index"`
	Kind  string       `json:"kind"`
	Hash  FieldElement `json:"hash"`
}

type LiabilitiesVector struct {
	Base  uint64 `json:"base"`
	Quote uint64 `json:"quote"`
}

type GenesisVectors struct {
	Accounts    []LeafVector      `json:"accounts"`
	AMM         LeafVector        `json:"amm"`
	Metadata    LeafVector        `json:"metadata"`
	EmptyLeaves []LeafVector      `json:"emptyLeaves"`
	Liabilities LiabilitiesVector `json:"liabilities"`
	Root        FieldElement      `json:"root"`
}

type MerkleProofVector struct {
	Index    int            `json:"index"`
	Leaf     FieldElement   `json:"leaf"`
	Siblings []FieldElement `json:"siblings"`
	Root     FieldElement   `json:"root"`
}

type MessageVector struct {
	MessageType     uint8  `json:"messageType"`
	MessageTypeName string `json:"messageTypeName"`
	SequenceID      uint64 `json:"sequenceId"`
	AccountID       uint64 `json:"accountId"`
	TokenID         uint8  `json:"tokenId"`
	TokenIDName     string `json:"tokenIdName"`
	DepositAmount   uint64 `json:"depositAmount"`
	Side            uint8  `json:"side"`
	SideName        string `json:"sideName"`
	BaseAmount      uint64 `json:"baseAmount"`
	LimitTick       uint64 `json:"limitTick"`
}

type BatchVector struct {
	BatchIndex          uint64          `json:"batchIndex"`
	StartSequenceID     uint64          `json:"startSequenceId"`
	MessageCount        uint8           `json:"messageCount"`
	Slots               []MessageVector `json:"slots"`
	CanonicalBytes      string          `json:"canonicalBytes"`
	KeccakDigest        string          `json:"keccakDigest"`
	CommitmentHi        string          `json:"commitmentHi"`
	CommitmentHiDecimal string          `json:"commitmentHiDecimal"`
	CommitmentLo        string          `json:"commitmentLo"`
	CommitmentLoDecimal string          `json:"commitmentLoDecimal"`
}

type BatchVectors struct {
	Full    BatchVector `json:"full"`
	Partial BatchVector `json:"partial"`
}

type GoldenVectors struct {
	Version     int               `json:"version"`
	Domains     DomainVectors     `json:"domains"`
	Genesis     GenesisVectors    `json:"genesis"`
	MerkleProof MerkleProofVector `json:"merkleProof"`
	Batches     BatchVectors      `json:"batches"`
}

func Build() (GoldenVectors, error) {
	domains := state.Domains()
	batchDomain := protocol.BatchDomainSeparator()
	result := GoldenVectors{
		Version: GoldenVectorSchemaVersion,
		Domains: DomainVectors{
			BatchEncodingVersion:   protocol.BatchEncodingVersion,
			BatchDomainSeparator:   hexBytes(batchDomain[:]),
			Poseidon2Parameters:    state.Poseidon2Parameters,
			Poseidon2DomainVersion: domains.Version,
			AccountLeaf:            fieldElement(domains.AccountLeaf),
			AMMLeaf:                fieldElement(domains.AMMLeaf),
			MetadataLeaf:           fieldElement(domains.MetadataLeaf),
			EmptyLeaf:              fieldElement(domains.EmptyLeaf),
			InternalNode:           fieldElement(domains.InternalNode),
		},
	}

	genesis := state.GenesisFixture()
	leaves, err := genesis.Leaves()
	if err != nil {
		return GoldenVectors{}, fmt.Errorf("build genesis leaves: %w", err)
	}
	tree, err := state.BuildTree(leaves[:])
	if err != nil {
		return GoldenVectors{}, fmt.Errorf("build genesis tree: %w", err)
	}
	root := tree.Root()
	liabilities, err := genesis.Liabilities()
	if err != nil {
		return GoldenVectors{}, fmt.Errorf("calculate genesis liabilities: %w", err)
	}

	result.Genesis.Accounts = make([]LeafVector, protocol.AccountLeafCount)
	for index := 0; index < protocol.AccountLeafCount; index++ {
		result.Genesis.Accounts[index] = LeafVector{
			Index: index,
			Kind:  "ACCOUNT",
			Hash:  fieldElement(leaves[index]),
		}
	}
	result.Genesis.AMM = LeafVector{
		Index: protocol.AMMLeafIndex,
		Kind:  "AMM",
		Hash:  fieldElement(leaves[protocol.AMMLeafIndex]),
	}
	result.Genesis.Metadata = LeafVector{
		Index: protocol.MetadataLeafIndex,
		Kind:  "METADATA",
		Hash:  fieldElement(leaves[protocol.MetadataLeafIndex]),
	}
	for index := protocol.FirstEmptyLeafIndex; index < protocol.LeafCount; index++ {
		result.Genesis.EmptyLeaves = append(result.Genesis.EmptyLeaves, LeafVector{
			Index: index,
			Kind:  "EMPTY",
			Hash:  fieldElement(leaves[index]),
		})
	}
	result.Genesis.Liabilities = LiabilitiesVector{
		Base:  liabilities.Base,
		Quote: liabilities.Quote,
	}
	result.Genesis.Root = fieldElement(root)

	proof, err := tree.Proof(0)
	if err != nil {
		return GoldenVectors{}, fmt.Errorf("build Merkle proof vector: %w", err)
	}
	result.MerkleProof = MerkleProofVector{
		Index:    proof.Index,
		Leaf:     fieldElement(leaves[proof.Index]),
		Siblings: make([]FieldElement, protocol.TreeDepth),
		Root:     fieldElement(root),
	}
	for level := range proof.Siblings {
		result.MerkleProof.Siblings[level] = fieldElement(proof.Siblings[level])
	}

	result.Batches.Full, err = batchVector(protocol.FullBatchFixture())
	if err != nil {
		return GoldenVectors{}, fmt.Errorf("build full-batch vector: %w", err)
	}
	result.Batches.Partial, err = batchVector(protocol.PartialBatchFixture())
	if err != nil {
		return GoldenVectors{}, fmt.Errorf("build partial-batch vector: %w", err)
	}
	return result, nil
}

func Write(path string) error {
	encoded, err := Marshal()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create golden vector directory: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write golden vectors: %w", err)
	}
	return nil
}

func Marshal() ([]byte, error) {
	vectors, err := Build()
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(vectors, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal golden vectors: %w", err)
	}
	return append(encoded, '\n'), nil
}

func fieldElement(element fr.Element) FieldElement {
	encoded := element.Bytes()
	return FieldElement{
		Decimal: element.String(),
		Hex:     hexBytes(encoded[:]),
	}
}

func batchVector(batch protocol.Batch) (BatchVector, error) {
	encoded, err := protocol.EncodeBatch(batch)
	if err != nil {
		return BatchVector{}, err
	}
	commitment, err := protocol.CommitBatch(batch)
	if err != nil {
		return BatchVector{}, err
	}

	vector := BatchVector{
		BatchIndex:          batch.BatchIndex,
		StartSequenceID:     batch.StartSequenceID,
		MessageCount:        batch.MessageCount,
		Slots:               make([]MessageVector, protocol.MaxSlots),
		CanonicalBytes:      hexBytes(encoded),
		KeccakDigest:        hexBytes(commitment.Digest[:]),
		CommitmentHi:        hexBytes(commitment.Hi[:]),
		CommitmentHiDecimal: new(big.Int).SetBytes(commitment.Hi[:]).String(),
		CommitmentLo:        hexBytes(commitment.Lo[:]),
		CommitmentLoDecimal: new(big.Int).SetBytes(commitment.Lo[:]).String(),
	}
	for index, message := range batch.Slots {
		vector.Slots[index] = MessageVector{
			MessageType:     uint8(message.Type),
			MessageTypeName: message.Type.String(),
			SequenceID:      message.SequenceID,
			AccountID:       message.AccountID,
			TokenID:         uint8(message.TokenID),
			TokenIDName:     message.TokenID.String(),
			DepositAmount:   message.DepositAmount,
			Side:            uint8(message.Side),
			SideName:        message.Side.String(),
			BaseAmount:      message.BaseAmount,
			LimitTick:       message.LimitTick,
		}
	}
	return vector, nil
}

func hexBytes(value []byte) string {
	return "0x" + hex.EncodeToString(value)
}
