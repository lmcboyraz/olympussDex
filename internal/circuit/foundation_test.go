package circuit

import (
	"math"
	"math/big"
	"testing"

	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/test"
)

type arithmeticHarness struct {
	Left       frontend.Variable
	Right      frontend.Variable
	Sum        frontend.Variable `gnark:",public"`
	Difference frontend.Variable `gnark:",public"`
	Product    frontend.Variable `gnark:",public"`
}

func (circuit *arithmeticHarness) Define(api frontend.API) error {
	g := NewGadgets(api)
	g.AssertUnsigned(circuit.Left, 64)
	g.AssertUnsigned(circuit.Right, 64)
	sum := g.AddNoOverflow(circuit.Left, circuit.Right, 64)
	difference := g.SubNoUnderflow(circuit.Left, circuit.Right, 64)
	product := g.MulNoOverflow(circuit.Left, circuit.Right, 64)
	api.AssertIsEqual(sum, circuit.Sum)
	api.AssertIsEqual(difference, circuit.Difference)
	api.AssertIsEqual(product, circuit.Product)
	return nil
}

func TestIntegerGadgetsRejectOverflowUnderflowAndWraparound(t *testing.T) {
	assert := test.NewAssert(t)
	circuit := &arithmeticHarness{}
	assert.SolvingSucceeded(circuit, &arithmeticHarness{
		Left:       9,
		Right:      3,
		Sum:        12,
		Difference: 6,
		Product:    27,
	}, test.WithCurves(ecc.BN254))
	assert.SolvingFailed(circuit, &arithmeticHarness{
		Left:       uint64(math.MaxUint64),
		Right:      1,
		Sum:        new(big.Int).Lsh(big.NewInt(1), 64),
		Difference: uint64(math.MaxUint64 - 1),
		Product:    uint64(math.MaxUint64),
	}, test.WithCurves(ecc.BN254))
	assert.SolvingFailed(circuit, &arithmeticHarness{
		Left:       1,
		Right:      2,
		Sum:        3,
		Difference: new(big.Int).Sub(ecc.BN254.ScalarField(), big.NewInt(1)),
		Product:    2,
	}, test.WithCurves(ecc.BN254))
	assert.SolvingFailed(circuit, &arithmeticHarness{
		Left:       uint64(math.MaxUint64),
		Right:      2,
		Sum:        0,
		Difference: uint64(math.MaxUint64 - 2),
		Product:    uint64(math.MaxUint64 - 1),
	}, test.WithCurves(ecc.BN254))
}

type stateRootHarness struct {
	State StateWitness
	Root  frontend.Variable `gnark:",public"`
}

func (circuit *stateRootHarness) Define(api frontend.API) error {
	root, err := ConstrainStateRoot(api, circuit.State)
	if err != nil {
		return err
	}
	api.AssertIsEqual(root, circuit.Root)
	return nil
}

func TestStateRootCircuitMatchesNativePoseidon2AndBindsEveryLeaf(t *testing.T) {
	pre := state.GenesisFixture()
	root, err := state.GenesisRoot()
	if err != nil {
		t.Fatal(err)
	}
	var rootBig big.Int
	root.BigInt(&rootBig)

	good := &stateRootHarness{State: AssignState(pre), Root: &rootBig}
	if err := test.IsSolved(&stateRootHarness{}, good, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("valid state root: %v", err)
	}

	wrongRoot := *good
	wrongRoot.Root = new(big.Int).Add(&rootBig, big.NewInt(1))
	if err := test.IsSolved(&stateRootHarness{}, &wrongRoot, ecc.BN254.ScalarField()); err == nil {
		t.Fatal("wrong root unexpectedly satisfied")
	}

	wrongState := *good
	wrongState.State.Accounts[3].QuoteBalance = 20_001
	if err := test.IsSolved(&stateRootHarness{}, &wrongState, ecc.BN254.ScalarField()); err == nil {
		t.Fatal("changed account unexpectedly satisfied against the old root")
	}
}

type batchBindingHarness struct {
	Batch          BatchWitness
	BatchIndex     frontend.Variable `gnark:",public"`
	MessageCount   frontend.Variable `gnark:",public"`
	CommitmentHigh frontend.Variable `gnark:",public"`
	CommitmentLow  frontend.Variable `gnark:",public"`
}

func (circuit *batchBindingHarness) Define(api frontend.API) error {
	return ConstrainBatchBinding(
		api,
		circuit.Batch,
		circuit.BatchIndex,
		circuit.MessageCount,
		circuit.CommitmentHigh,
		circuit.CommitmentLow,
	)
}

func TestBatchCircuitMatchesCanonicalKeccakCommitment(t *testing.T) {
	for name, batch := range map[string]protocol.Batch{
		"full":    protocol.FullBatchFixture(),
		"partial": protocol.PartialBatchFixture(),
	} {
		t.Run(name, func(t *testing.T) {
			commitment, err := protocol.CommitBatch(batch)
			if err != nil {
				t.Fatal(err)
			}
			assignment := batchHarnessAssignment(batch, commitment)
			if err := test.IsSolved(&batchBindingHarness{}, assignment, ecc.BN254.ScalarField()); err != nil {
				t.Fatalf("canonical batch: %v", err)
			}

			wrongHigh := *assignment
			wrongHigh.CommitmentHigh = new(big.Int).Add(
				new(big.Int).SetBytes(commitment.Hi[:]),
				big.NewInt(1),
			)
			if err := test.IsSolved(&batchBindingHarness{}, &wrongHigh, ecc.BN254.ScalarField()); err == nil {
				t.Fatal("wrong commitment high half unexpectedly satisfied")
			}

			wrongBatch := *assignment
			wrongBatch.Batch.Slots[0].SequenceID = 99
			if err := test.IsSolved(&batchBindingHarness{}, &wrongBatch, ecc.BN254.ScalarField()); err == nil {
				t.Fatal("changed private batch unexpectedly satisfied")
			}
		})
	}
}

func TestBatchCircuitRejectsEveryCanonicalShapeViolation(t *testing.T) {
	base := protocol.PartialBatchFixture()
	tests := map[string]func(*protocol.Batch){
		"batch index above 61 bits": func(batch *protocol.Batch) {
			batch.BatchIndex = protocol.MaxBatchIndex + 1
		},
		"zero message count": func(batch *protocol.Batch) {
			batch.MessageCount = 0
		},
		"empty in prefix": func(batch *protocol.Batch) {
			batch.MessageCount = 2
		},
		"real in suffix": func(batch *protocol.Batch) {
			batch.Slots[7] = batch.Slots[0]
			batch.Slots[7].SequenceID = 106
		},
		"noncanonical empty": func(batch *protocol.Batch) {
			batch.Slots[1].LimitTick = 1
		},
		"invalid message enum": func(batch *protocol.Batch) {
			batch.Slots[0].Type = 3
		},
		"invalid account": func(batch *protocol.Batch) {
			batch.Slots[0].AccountID = 8
		},
		"invalid token": func(batch *protocol.Batch) {
			batch.Slots[0].TokenID = 2
		},
		"zero deposit": func(batch *protocol.Batch) {
			batch.Slots[0].DepositAmount = 0
		},
		"deposit union dirt": func(batch *protocol.Batch) {
			batch.Slots[0].BaseAmount = 1
		},
		"wrong sequence": func(batch *protocol.Batch) {
			batch.Slots[0].SequenceID++
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			batch := base
			mutate(&batch)
			assignment := batchHarnessAssignment(batch, mustCommitment(t, base))
			if err := test.IsSolved(&batchBindingHarness{}, assignment, ecc.BN254.ScalarField()); err == nil {
				t.Fatal("malformed batch unexpectedly satisfied")
			}
		})
	}
}

func batchHarnessAssignment(
	batch protocol.Batch,
	commitment protocol.Commitment,
) *batchBindingHarness {
	return &batchBindingHarness{
		Batch:          AssignBatch(batch),
		BatchIndex:     batch.BatchIndex,
		MessageCount:   batch.MessageCount,
		CommitmentHigh: new(big.Int).SetBytes(commitment.Hi[:]),
		CommitmentLow:  new(big.Int).SetBytes(commitment.Lo[:]),
	}
}

func mustCommitment(t *testing.T, batch protocol.Batch) protocol.Commitment {
	t.Helper()
	commitment, err := protocol.CommitBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	return commitment
}
