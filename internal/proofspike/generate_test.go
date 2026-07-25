package proofspike

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/cemilboyraz/oly2/internal/publicinputs"
)

func TestSolidityCallDataUsesGnarkPublicWitnessInCanonicalOrder(t *testing.T) {
	fixture := newGroth16TestFixture(t)

	callData, err := buildSolidityCallData(fixture.proof, fixture.publicWitness)
	if err != nil {
		t.Fatalf("build Solidity calldata: %v", err)
	}
	names := publicinputs.Names()
	if len(callData.PublicInputs) != publicinputs.Count {
		t.Fatalf(
			"Solidity public input length = %d, want %d",
			len(callData.PublicInputs),
			publicinputs.Count,
		)
	}
	for i, input := range callData.PublicInputs {
		if input.Index != i {
			t.Errorf("input %d index = %d", i, input.Index)
		}
		if input.Name != names[i] {
			t.Errorf("input %d name = %q, want %q", i, input.Name, names[i])
		}
		if input.Value != fixture.assignment.Inputs[i].String() {
			t.Errorf(
				"input %d (%s) value = %s, want %s",
				i,
				input.Name,
				input.Value,
				fixture.assignment.Inputs[i],
			)
		}
	}
}

func TestGeneratedVerifierDeclaresExactly27Inputs(t *testing.T) {
	config := GenerateConfig{
		ArtifactDirectory: t.TempDir(),
		SolidityVerifier:  t.TempDir() + "/Verifier.sol",
	}
	result, err := Generate(config)
	if err != nil {
		t.Fatalf("generate proof spike: %v", err)
	}

	// This also guards the generated verifier ABI. An ABI drift here would make
	// the Anvil invocation disagree with the canonical Go definition.
	source, err := os.ReadFile(config.SolidityVerifier)
	if err != nil {
		t.Fatalf("read generated verifier: %v", err)
	}
	want := []byte(fmt.Sprintf(
		"uint256[%d] calldata input",
		publicinputs.Count,
	))
	if !bytes.Contains(source, want) {
		t.Fatalf("generated verifier does not contain %q", want)
	}
	if len(result.CallData.PublicInputs) != publicinputs.Count {
		t.Fatalf("generated calldata has %d public inputs", len(result.CallData.PublicInputs))
	}
}
