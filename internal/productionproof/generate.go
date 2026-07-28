// Package productionproof runs the disposable Groth16/Solidity pipeline for
// the production FIFO-Clear circuit. It is intentionally separate from the
// Milestone 0 proof-spike package.
package productionproof

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/cemilboyraz/oly2/internal/circuit"
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend"
	"github.com/consensys/gnark/backend/groth16"
	groth16bn254 "github.com/consensys/gnark/backend/groth16/bn254"
	"github.com/consensys/gnark/backend/solidity"
	backendwitness "github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

const (
	r1csFilename          = "fifo-clear.r1cs"
	provingKeyFilename    = "proving.key"
	verifyingKeyFilename  = "verifying.key"
	witnessFilename       = "witness.bin"
	proofFilename         = "proof.bin"
	publicWitnessFilename = "public-witness.bin"
	callDataFilename      = "calldata.json"
)

// GenerateConfig selects a canonical transition and disposable output paths.
type GenerateConfig struct {
	ArtifactDirectory string
	SolidityVerifier  string
	PreState          state.State
	Batch             protocol.Batch
}

// Compiled reports the auditable production circuit shape.
type Compiled struct {
	ConstraintCount  int
	PublicInputCount int
	SecretInputCount int
	system           constraint.ConstraintSystem
}

// Result reports proof-pipeline outputs without retaining proving material in
// memory after the command exits.
type Result struct {
	ConstraintCount             int
	PublicInputCount            int
	SecretInputCount            int
	TamperedPublicInputRejected bool
	ArtifactPaths               []string
	CallData                    SolidityCallData
}

// SolidityCallData is the exact generated verifier call payload.
type SolidityCallData struct {
	Proof         [8]string       `json:"proof"`
	Commitments   []string        `json:"commitments"`
	CommitmentPoK [2]string       `json:"commitmentPok"`
	PublicInputs  []SolidityInput `json:"publicInputs"`
}

// SolidityInput retains canonical index/name metadata beside each scalar.
type SolidityInput struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Compile creates the production BN254 R1CS and exposes its explicit input
// counts. gnark includes its constant-one wire in GetNbPublicVariables.
func Compile() (Compiled, error) {
	system, err := frontend.Compile(
		ecc.BN254.ScalarField(),
		r1cs.NewBuilder,
		&circuit.Circuit{},
	)
	if err != nil {
		return Compiled{}, fmt.Errorf("compile production circuit: %w", err)
	}
	return Compiled{
		ConstraintCount:  system.GetNbConstraints(),
		PublicInputCount: system.GetNbPublicVariables() - 1,
		SecretInputCount: system.GetNbSecretVariables(),
		system:           system,
	}, nil
}

// Generate compiles, sets up, witnesses, proves, verifies, checks a tampered
// public input, exports Solidity, and writes only ignored build artifacts.
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

	compiled, err := Compile()
	if err != nil {
		return Result{}, err
	}
	if compiled.PublicInputCount != publicinputs.Count {
		return Result{}, fmt.Errorf(
			"compiled public input count = %d, want %d",
			compiled.PublicInputCount,
			publicinputs.Count,
		)
	}
	provingKey, verifyingKey, err := groth16.Setup(compiled.system)
	if err != nil {
		return Result{}, fmt.Errorf("production Groth16 setup: %w", err)
	}

	assignment, _, err := circuit.NewAssignment(config.PreState, config.Batch)
	if err != nil {
		return Result{}, fmt.Errorf("build production assignment: %w", err)
	}
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return Result{}, fmt.Errorf("create production witness: %w", err)
	}
	publicWitness, err := witness.Public()
	if err != nil {
		return Result{}, fmt.Errorf("extract production public witness: %w", err)
	}
	if vector, ok := publicWitness.Vector().(fr.Vector); !ok || len(vector) != publicinputs.Count {
		return Result{}, fmt.Errorf(
			"production public witness has unexpected vector %T length",
			publicWitness.Vector(),
		)
	}

	proof, err := groth16.Prove(
		compiled.system,
		provingKey,
		witness,
		backend.WithProverHashToFieldFunction(sha256.New()),
	)
	if err != nil {
		return Result{}, fmt.Errorf("produce production Groth16 proof: %w", err)
	}
	if err := groth16.Verify(
		proof,
		verifyingKey,
		publicWitness,
		backend.WithVerifierHashToFieldFunction(sha256.New()),
	); err != nil {
		return Result{}, fmt.Errorf("verify production Groth16 proof in Go: %w", err)
	}
	rejected, err := rejectsTamperedPublicInput(proof, verifyingKey, assignment)
	if err != nil {
		return Result{}, err
	}
	if !rejected {
		return Result{}, fmt.Errorf("production proof verified with a tampered public input")
	}

	callData, err := buildSolidityCallData(proof, publicWitness)
	if err != nil {
		return Result{}, err
	}
	paths := []string{
		filepath.Join(config.ArtifactDirectory, r1csFilename),
		filepath.Join(config.ArtifactDirectory, provingKeyFilename),
		filepath.Join(config.ArtifactDirectory, verifyingKeyFilename),
		filepath.Join(config.ArtifactDirectory, witnessFilename),
		filepath.Join(config.ArtifactDirectory, proofFilename),
		filepath.Join(config.ArtifactDirectory, publicWitnessFilename),
		filepath.Join(config.ArtifactDirectory, callDataFilename),
		config.SolidityVerifier,
	}
	writers := []io.WriterTo{
		compiled.system,
		provingKey,
		verifyingKey,
		witness,
		proof,
		publicWitness,
	}
	for index, writer := range writers {
		if err := writeBinary(paths[index], writer); err != nil {
			return Result{}, err
		}
	}
	if err := writeJSON(paths[6], callData); err != nil {
		return Result{}, err
	}
	if err := exportSolidity(paths[7], verifyingKey); err != nil {
		return Result{}, err
	}

	return Result{
		ConstraintCount:             compiled.ConstraintCount,
		PublicInputCount:            compiled.PublicInputCount,
		SecretInputCount:            compiled.SecretInputCount,
		TamperedPublicInputRejected: rejected,
		ArtifactPaths:               paths,
		CallData:                    callData,
	}, nil
}

func rejectsTamperedPublicInput(
	proof groth16.Proof,
	verifyingKey groth16.VerifyingKey,
	assignment *circuit.Circuit,
) (bool, error) {
	tampered := *assignment
	original, ok := assignment.Public.Inputs[publicinputs.BatchIndex].(*big.Int)
	if !ok {
		return false, fmt.Errorf(
			"batchIndex assignment has unexpected type %T",
			assignment.Public.Inputs[publicinputs.BatchIndex],
		)
	}
	tampered.Public.Inputs[publicinputs.BatchIndex] = new(big.Int).Add(
		new(big.Int).Set(original),
		big.NewInt(1),
	)
	witness, err := frontend.NewWitness(&tampered, ecc.BN254.ScalarField())
	if err != nil {
		return false, fmt.Errorf("create tampered public witness: %w", err)
	}
	publicWitness, err := witness.Public()
	if err != nil {
		return false, fmt.Errorf("extract tampered public witness: %w", err)
	}
	return groth16.Verify(
		proof,
		verifyingKey,
		publicWitness,
		backend.WithVerifierHashToFieldFunction(sha256.New()),
	) != nil, nil
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
	coreLength := len(SolidityCallData{}.Proof) * fr.Bytes
	commitmentLength := len(bn254Proof.Commitments) * 2 * fr.Bytes
	expectedLength := coreLength
	if len(bn254Proof.Commitments) > 0 {
		expectedLength += 4 + commitmentLength + 2*fr.Bytes
	}
	if len(proofBytes) != expectedLength {
		return SolidityCallData{}, fmt.Errorf(
			"Solidity proof length = %d bytes, want %d",
			len(proofBytes),
			expectedLength,
		)
	}

	var callData SolidityCallData
	for index := range callData.Proof {
		word := proofBytes[index*fr.Bytes : (index+1)*fr.Bytes]
		callData.Proof[index] = new(big.Int).SetBytes(word).String()
	}
	if len(bn254Proof.Commitments) > 0 {
		// WriteRawTo prefixes the commitment slice with its uint32 length. The
		// generated Solidity ABI expects only the following affine coordinates.
		commitmentStart := coreLength + 4
		callData.Commitments = make([]string, len(bn254Proof.Commitments)*2)
		for index := range callData.Commitments {
			start := commitmentStart + index*fr.Bytes
			callData.Commitments[index] = new(big.Int).SetBytes(
				proofBytes[start : start+fr.Bytes],
			).String()
		}
		pokStart := commitmentStart + commitmentLength
		for index := range callData.CommitmentPoK {
			start := pokStart + index*fr.Bytes
			callData.CommitmentPoK[index] = new(big.Int).SetBytes(
				proofBytes[start : start+fr.Bytes],
			).String()
		}
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
	for index := range vector {
		var value big.Int
		vector[index].BigInt(&value)
		callData.PublicInputs[index] = SolidityInput{
			Index: index,
			Name:  names[index],
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
		return verifyingKey.ExportSolidity(
			writer,
			solidity.WithHashToFieldFunction(sha256.New()),
		)
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
