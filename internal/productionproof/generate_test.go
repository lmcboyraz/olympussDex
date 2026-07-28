package productionproof

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cemilboyraz/oly2/internal/engine"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
)

func TestProductionCircuitSchemaHasOnlyCanonicalInputs(t *testing.T) {
	compiled, err := Compile()
	if err != nil {
		t.Fatal(err)
	}
	if compiled.PublicInputCount != publicinputs.Count {
		t.Fatalf(
			"public inputs = %d, want %d",
			compiled.PublicInputCount,
			publicinputs.Count,
		)
	}
	const explicitPrivateInputs = 87
	if compiled.SecretInputCount != explicitPrivateInputs {
		t.Fatalf(
			"private inputs = %d, want %d pre-state/batch fields only",
			compiled.SecretInputCount,
			explicitPrivateInputs,
		)
	}
	if compiled.ConstraintCount <= 0 {
		t.Fatalf("constraint count = %d", compiled.ConstraintCount)
	}
}

func TestGenerateRunsProductionGroth16AndExactSolidityABI(t *testing.T) {
	artifactDirectory := t.TempDir()
	verifierPath := filepath.Join(t.TempDir(), "ProductionVerifier.sol")
	result, err := Generate(GenerateConfig{
		ArtifactDirectory: artifactDirectory,
		SolidityVerifier:  verifierPath,
		PreState:          state.GenesisFixture(),
		Batch:             engine.ShowcaseBatch0(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PublicInputCount != publicinputs.Count {
		t.Fatalf("public inputs = %d", result.PublicInputCount)
	}
	if result.ConstraintCount <= 0 {
		t.Fatalf("constraint count = %d", result.ConstraintCount)
	}
	if !result.TamperedPublicInputRejected {
		t.Fatal("native verifier did not reject the tampered public input")
	}
	if len(result.CallData.Commitments) == 0 {
		t.Fatal("production proof unexpectedly omitted lookup commitments")
	}
	for _, path := range result.ArtifactPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("artifact %s: %v", path, err)
		}
	}

	source, err := os.ReadFile(verifierPath)
	if err != nil {
		t.Fatal(err)
	}
	wantABI := []byte(fmt.Sprintf("uint256[%d] calldata input", publicinputs.Count))
	if !bytes.Contains(source, wantABI) {
		t.Fatalf("generated verifier does not contain %q", wantABI)
	}
	wantCommitmentsABI := []byte(fmt.Sprintf(
		"uint256[%d] calldata commitments",
		len(result.CallData.Commitments),
	))
	if !bytes.Contains(source, wantCommitmentsABI) {
		t.Fatalf("generated verifier does not contain %q", wantCommitmentsABI)
	}

	execution, err := engine.Execute(state.GenesisFixture(), engine.ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	values := execution.PublicInputs()
	names := publicinputs.Names()
	if len(result.CallData.PublicInputs) != publicinputs.Count {
		t.Fatalf("calldata inputs = %d", len(result.CallData.PublicInputs))
	}
	for index, input := range result.CallData.PublicInputs {
		if input.Index != index ||
			input.Name != names[index] ||
			input.Value != values[index].String() {
			t.Fatalf(
				"calldata[%d] = %+v, want name=%s value=%s",
				index,
				input,
				names[index],
				values[index],
			)
		}
	}
}
