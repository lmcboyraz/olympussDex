package proofspike

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
)

func TestGroth16RoundTripAndRejectsMutatedPublicInput(t *testing.T) {
	fixture := newGroth16TestFixture(t)
	publicWitness := fixture.publicWitness
	if got := reflect.ValueOf(publicWitness.Vector()).Len(); got != publicinputs.Count {
		t.Fatalf("public witness length = %d, want %d", got, publicinputs.Count)
	}

	if err := groth16.Verify(fixture.proof, fixture.verifyingKey, publicWitness); err != nil {
		t.Fatalf("verify valid proof: %v", err)
	}

	names := publicinputs.Names()
	for i := range publicinputs.Count {
		mutated := fixture.assignment
		mutated.Inputs[i] = new(big.Int).Add(
			fixture.assignment.Inputs[i],
			big.NewInt(1),
		)
		mutatedAssignment, err := mutated.Circuit()
		if err != nil {
			t.Fatalf("build assignment with mutated %s: %v", names[i], err)
		}
		mutatedWitness, err := frontend.NewWitness(
			mutatedAssignment,
			ecc.BN254.ScalarField(),
		)
		if err != nil {
			t.Fatalf("create witness with mutated %s: %v", names[i], err)
		}
		mutatedPublicWitness, err := mutatedWitness.Public()
		if err != nil {
			t.Fatalf("extract witness with mutated %s: %v", names[i], err)
		}
		if err := groth16.Verify(fixture.proof, fixture.verifyingKey, mutatedPublicWitness); err == nil {
			t.Fatalf("mutated public input %d (%s) unexpectedly verified", i, names[i])
		}
	}
}
