package state

import "github.com/consensys/gnark-crypto/ecc/bn254/fr"

const (
	Poseidon2DomainVersion = 1

	accountLeafDomainValue  uint64 = 0x0101
	ammLeafDomainValue      uint64 = 0x0102
	metadataLeafDomainValue uint64 = 0x0103
	emptyLeafDomainValue    uint64 = 0x0104
	internalNodeDomainValue uint64 = 0x0105

	Poseidon2Parameters = "bn254/fr/poseidon2.NewMerkleDamgardHasher; Poseidon2-BN254[t=2,rF=6,rP=50,d=5]"
)

type DomainConstants struct {
	Version      uint64
	AccountLeaf  fr.Element
	AMMLeaf      fr.Element
	MetadataLeaf fr.Element
	EmptyLeaf    fr.Element
	InternalNode fr.Element
}

// Domains returns copies of the explicit v1 field elements used by every state
// hash. The high byte is the version and the low byte is the domain kind.
func Domains() DomainConstants {
	return DomainConstants{
		Version:      Poseidon2DomainVersion,
		AccountLeaf:  fr.NewElement(accountLeafDomainValue),
		AMMLeaf:      fr.NewElement(ammLeafDomainValue),
		MetadataLeaf: fr.NewElement(metadataLeafDomainValue),
		EmptyLeaf:    fr.NewElement(emptyLeafDomainValue),
		InternalNode: fr.NewElement(internalNodeDomainValue),
	}
}
