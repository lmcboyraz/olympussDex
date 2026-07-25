package proofspike

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend/groth16"
	groth16bn254 "github.com/consensys/gnark/backend/groth16/bn254"
	backendwitness "github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

const (
	r1csFilename          = "circuit.r1cs"
	provingKeyFilename    = "proving.key"
	verifyingKeyFilename  = "verifying.key"
	proofFilename         = "proof.bin"
	publicWitnessFilename = "public-witness.bin"
	callDataFilename      = "calldata.json"
)

type GenerateConfig struct {
	ArtifactDirectory string
	SolidityVerifier  string
}

type Result struct {
	ConstraintCount int
	ArtifactPaths   []string
	CallData        SolidityCallData
}

type SolidityCallData struct {
	Proof        [8]string       `json:"proof"`
	PublicInputs []SolidityInput `json:"publicInputs"`
}

type SolidityInput struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Generate executes the complete native proof spike and writes only disposable
// build products. The caller supplies both artifact destinations so generated
// cryptographic material never needs to live in source-controlled paths.
func Generate(config GenerateConfig) (Result, error) {
	if config.ArtifactDirectory == "" {
		return Result{}, fmt.Errorf("artifact directory is required")
	}
	if config.SolidityVerifier == "" {
		return Result{}, fmt.Errorf("Solidity verifier path is required")
	}
	if err := os.MkdirAll(config.ArtifactDirectory, 0o755); err != nil {
		return Result{}, fmt.Errorf("create artifact directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(config.SolidityVerifier), 0o755); err != nil {
		return Result{}, fmt.Errorf("create Solidity verifier directory: %w", err)
	}

	constraintSystem, err := frontend.Compile(
		ecc.BN254.ScalarField(),
		r1cs.NewBuilder,
		&Circuit{},
	)
	if err != nil {
		return Result{}, fmt.Errorf("compile circuit: %w", err)
	}

	provingKey, verifyingKey, err := groth16.Setup(constraintSystem)
	if err != nil {
		return Result{}, fmt.Errorf("Groth16 setup: %w", err)
	}

	fixture, err := Fixture().Circuit()
	if err != nil {
		return Result{}, fmt.Errorf("build fixture: %w", err)
	}
	witness, err := frontend.NewWitness(fixture, ecc.BN254.ScalarField())
	if err != nil {
		return Result{}, fmt.Errorf("create witness: %w", err)
	}
	publicWitness, err := witness.Public()
	if err != nil {
		return Result{}, fmt.Errorf("extract public witness: %w", err)
	}

	proof, err := groth16.Prove(constraintSystem, provingKey, witness)
	if err != nil {
		return Result{}, fmt.Errorf("produce Groth16 proof: %w", err)
	}
	if err := groth16.Verify(proof, verifyingKey, publicWitness); err != nil {
		return Result{}, fmt.Errorf("verify Groth16 proof in Go: %w", err)
	}

	callData, err := buildSolidityCallData(proof, publicWitness)
	if err != nil {
		return Result{}, err
	}

	paths := []string{
		filepath.Join(config.ArtifactDirectory, r1csFilename),
		filepath.Join(config.ArtifactDirectory, provingKeyFilename),
		filepath.Join(config.ArtifactDirectory, verifyingKeyFilename),
		filepath.Join(config.ArtifactDirectory, proofFilename),
		filepath.Join(config.ArtifactDirectory, publicWitnessFilename),
		filepath.Join(config.ArtifactDirectory, callDataFilename),
		config.SolidityVerifier,
	}
	writers := []io.WriterTo{
		constraintSystem,
		provingKey,
		verifyingKey,
		proof,
		publicWitness,
	}
	for i, writer := range writers {
		if err := writeBinary(paths[i], writer); err != nil {
			return Result{}, err
		}
	}
	if err := writeJSON(paths[5], callData); err != nil {
		return Result{}, err
	}
	if err := exportSolidity(paths[6], verifyingKey); err != nil {
		return Result{}, err
	}

	return Result{
		ConstraintCount: constraintSystem.GetNbConstraints(),
		ArtifactPaths:   paths,
		CallData:        callData,
	}, nil
}

func buildSolidityCallData(
	proof groth16.Proof,
	publicWitness backendwitness.Witness,
) (SolidityCallData, error) {
	bn254Proof, ok := proof.(*groth16bn254.Proof)
	if !ok {
		return SolidityCallData{}, fmt.Errorf("expected BN254 proof, got %T", proof)
	}
	proofBytes := bn254Proof.MarshalSolidity()
	if len(proofBytes) != len(SolidityCallData{}.Proof)*fr.Bytes {
		return SolidityCallData{}, fmt.Errorf(
			"Solidity proof length = %d bytes, want %d",
			len(proofBytes),
			len(SolidityCallData{}.Proof)*fr.Bytes,
		)
	}

	var callData SolidityCallData
	for i := range callData.Proof {
		word := proofBytes[i*fr.Bytes : (i+1)*fr.Bytes]
		callData.Proof[i] = new(big.Int).SetBytes(word).String()
	}

	vector, ok := publicWitness.Vector().(fr.Vector)
	if !ok {
		return SolidityCallData{}, fmt.Errorf(
			"expected BN254 public witness vector, got %T",
			publicWitness.Vector(),
		)
	}
	if len(vector) != publicinputs.Count {
		return SolidityCallData{}, fmt.Errorf(
			"public witness length = %d, want %d",
			len(vector),
			publicinputs.Count,
		)
	}

	names := publicinputs.Names()
	callData.PublicInputs = make([]SolidityInput, publicinputs.Count)
	for i := range vector {
		var value big.Int
		vector[i].BigInt(&value)
		callData.PublicInputs[i] = SolidityInput{
			Index: i,
			Name:  names[i],
			Value: value.String(),
		}
	}
	return callData, nil
}

func writeBinary(path string, value io.WriterTo) error {
	return writeArtifact(path, func(writer io.Writer) error {
		_, err := value.WriteTo(writer)
		return err
	})
}

func writeJSON(path string, value any) error {
	return writeArtifact(path, func(writer io.Writer) error {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	})
}

func exportSolidity(path string, verifyingKey groth16.VerifyingKey) error {
	return writeArtifact(path, func(writer io.Writer) error {
		return verifyingKey.ExportSolidity(writer)
	})
}

func writeArtifact(path string, write func(io.Writer) error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	writeErr := write(file)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	return nil
}
