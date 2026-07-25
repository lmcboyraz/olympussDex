package proofspike

import (
	"fmt"
	"math/big"

	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/consensys/gnark/frontend"
)

// Circuit is deliberately a proof-of-toolchain circuit, not the final
// FIFO-Clear circuit. Its array is the one public gnark field, so gnark and the
// exported Solidity verifier both observe the canonical array order.
type Circuit struct {
	Inputs [publicinputs.Count]frontend.Variable `gnark:",public"`
	Secret frontend.Variable
}

// Define binds every public input into deterministic spike-only relationships.
// The first input is the square of the private secret; every later input is the
// previous public input plus its index.
func (circuit *Circuit) Define(api frontend.API) error {
	api.AssertIsEqual(
		circuit.Inputs[publicinputs.BatchIndex],
		api.Mul(circuit.Secret, circuit.Secret),
	)
	for i := 1; i < publicinputs.Count; i++ {
		api.AssertIsEqual(circuit.Inputs[i], api.Add(circuit.Inputs[i-1], i))
	}
	return nil
}

// Assignment contains the private spike secret and the canonical public vector.
type Assignment struct {
	Secret *big.Int
	Inputs [publicinputs.Count]*big.Int
}

// Fixture returns a deterministic, satisfiable assignment for local tests and
// the proof-spike command. This fixture has no production protocol semantics.
func Fixture() Assignment {
	assignment := Assignment{Secret: big.NewInt(42)}
	assignment.Inputs[publicinputs.BatchIndex] = new(big.Int).Mul(
		assignment.Secret,
		assignment.Secret,
	)
	for i := 1; i < publicinputs.Count; i++ {
		assignment.Inputs[i] = new(big.Int).Add(
			assignment.Inputs[i-1],
			big.NewInt(int64(i)),
		)
	}
	return assignment
}

func (assignment Assignment) Circuit() (*Circuit, error) {
	if assignment.Secret == nil {
		return nil, fmt.Errorf("secret is required")
	}

	circuit := &Circuit{Secret: new(big.Int).Set(assignment.Secret)}
	for i, value := range assignment.Inputs {
		if value == nil {
			return nil, fmt.Errorf("public input %q is required", publicinputs.Names()[i])
		}
		circuit.Inputs[i] = new(big.Int).Set(value)
	}
	return circuit, nil
}
