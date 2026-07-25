package proofspike

import (
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	backendwitness "github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

type groth16TestFixture struct {
	assignment    Assignment
	proof         groth16.Proof
	verifyingKey  groth16.VerifyingKey
	publicWitness backendwitness.Witness
}

func newGroth16TestFixture(t *testing.T) groth16TestFixture {
	t.Helper()

	constraintSystem, err := frontend.Compile(
		ecc.BN254.ScalarField(),
		r1cs.NewBuilder,
		&Circuit{},
	)
	if err != nil {
		t.Fatalf("compile circuit: %v", err)
	}
	provingKey, verifyingKey, err := groth16.Setup(constraintSystem)
	if err != nil {
		t.Fatalf("Groth16 setup: %v", err)
	}

	fixture := Fixture()
	assignment, err := fixture.Circuit()
	if err != nil {
		t.Fatalf("build assignment: %v", err)
	}
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("create witness: %v", err)
	}
	publicWitness, err := witness.Public()
	if err != nil {
		t.Fatalf("extract public witness: %v", err)
	}
	proof, err := groth16.Prove(constraintSystem, provingKey, witness)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	return groth16TestFixture{
		assignment:    fixture,
		proof:         proof,
		verifyingKey:  verifyingKey,
		publicWitness: publicWitness,
	}
}
