package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/cemilboyraz/oly2/internal/engine"
	"github.com/cemilboyraz/oly2/internal/productionproof"
	"github.com/cemilboyraz/oly2/internal/state"
)

func main() {
	artifactDirectory := flag.String(
		"output",
		"build/production-proof",
		"directory for disposable production proof artifacts",
	)
	solidityVerifier := flag.String(
		"solidity",
		"contracts/generated/ProductionVerifier.sol",
		"path for the generated production Solidity verifier",
	)
	flag.Parse()

	result, err := productionproof.Generate(productionproof.GenerateConfig{
		ArtifactDirectory: *artifactDirectory,
		SolidityVerifier:  *solidityVerifier,
		PreState:          state.GenesisFixture(),
		Batch:             engine.ShowcaseBatch0(),
	})
	if err != nil {
		log.Fatalf("production proof failed: %v", err)
	}

	fmt.Printf(
		"production circuit compile: PASS (%d constraints, %d public inputs, %d private inputs)\n",
		result.ConstraintCount,
		result.PublicInputCount,
		result.SecretInputCount,
	)
	fmt.Println("production Groth16 setup: PASS")
	fmt.Println("production witness creation: PASS")
	fmt.Println("production proof generation: PASS")
	fmt.Println("production native Go verification: PASS")
	fmt.Println("production tampered-public-input rejection: PASS")
	fmt.Println("production Solidity verifier generation: PASS")
	for _, path := range result.ArtifactPaths {
		fmt.Printf("artifact: %s\n", path)
	}
}
