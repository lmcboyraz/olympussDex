package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/cemilboyraz/oly2/internal/proofspike"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
)

func main() {
	artifactDirectory := flag.String(
		"output",
		"build/proof-spike",
		"directory for disposable proof artifacts",
	)
	solidityVerifier := flag.String(
		"solidity",
		"contracts/generated/Verifier.sol",
		"path for the generated Solidity verifier",
	)
	flag.Parse()

	result, err := proofspike.Generate(proofspike.GenerateConfig{
		ArtifactDirectory: *artifactDirectory,
		SolidityVerifier:  *solidityVerifier,
	})
	if err != nil {
		log.Fatalf("proof spike failed: %v", err)
	}

	fmt.Printf(
		"circuit compile: PASS (%d constraints, %d public inputs)\n",
		result.ConstraintCount,
		publicinputs.Count,
	)
	fmt.Println("Groth16 setup: PASS")
	fmt.Println("witness creation: PASS")
	fmt.Println("proof generation: PASS")
	fmt.Println("Go verification: PASS")
	fmt.Println("Solidity verifier generation: PASS")
	for _, path := range result.ArtifactPaths {
		fmt.Printf("artifact: %s\n", path)
	}
}
