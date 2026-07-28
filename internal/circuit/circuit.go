package circuit

import (
	"fmt"
	"math/big"

	"github.com/cemilboyraz/oly2/internal/engine"
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark/frontend"
)

// Circuit is the production FIFO-Clear transition circuit. Its only explicit
// private inputs are the pre-state and canonical batch; all execution
// auxiliaries are recomputed or obtained through fully constrained hints.
type Circuit struct {
	Public   PublicWitness
	PreState StateWitness
	Batch    BatchWitness
}

// Define independently recreates all M1 bindings and the complete M2
// transition, then binds every result to the canonical 27 public inputs.
func (circuit *Circuit) Define(api frontend.API) error {
	inputs := circuit.Public.Inputs
	flags, err := constrainBatchBinding(
		api,
		circuit.Batch,
		inputs[publicinputs.BatchIndex],
		inputs[publicinputs.MessageCount],
		inputs[publicinputs.BatchCommitmentHi],
		inputs[publicinputs.BatchCommitmentLo],
	)
	if err != nil {
		return fmt.Errorf("batch binding: %w", err)
	}

	oldRoot, err := ConstrainStateRoot(api, circuit.PreState)
	if err != nil {
		return fmt.Errorf("old state root: %w", err)
	}
	api.AssertIsEqual(oldRoot, inputs[publicinputs.OldStateRoot])

	result, err := constrainTransition(api, circuit.PreState, circuit.Batch, flags)
	if err != nil {
		return fmt.Errorf("FIFO-Clear transition: %w", err)
	}
	newRoot, err := ConstrainStateRoot(api, result.postState)
	if err != nil {
		return fmt.Errorf("new state root: %w", err)
	}
	api.AssertIsEqual(newRoot, inputs[publicinputs.NewStateRoot])
	constrainPublicTrace(api, result, inputs)
	return nil
}

// NewAssignment executes the reference engine only to populate witness values.
// Circuit.Define does not consume the returned engine trace as private advice;
// it independently derives and constrains the same transition.
func NewAssignment(
	pre state.State,
	batch protocol.Batch,
) (*Circuit, engine.Result, error) {
	result, err := engine.Execute(pre, batch)
	if err != nil {
		return nil, engine.Result{}, err
	}
	assignment := &Circuit{
		PreState: AssignState(pre),
		Batch:    AssignBatch(batch),
	}
	values := result.PublicInputs()
	for index, value := range values {
		assignment.Public.Inputs[index] = new(big.Int).Set(value)
	}
	return assignment, result, nil
}

func constrainPublicTrace(
	api frontend.API,
	result transitionResult,
	inputs [publicinputs.Count]frontend.Variable,
) {
	g := NewGadgets(api)
	g.AssertUnsigned(inputs[publicinputs.ClearingTick], 8)
	api.AssertIsEqual(inputs[publicinputs.ClearingTick], result.clearingTick)
	for slot := 0; slot < protocol.MaxSlots; slot++ {
		status := inputs[publicinputs.FundingStatus(slot)]
		g.AssertUnsigned(status, 2)
		g.AssertEnum(status, 0, 1, 2)
		api.AssertIsEqual(status, result.fundingStatus[slot])

		fill := inputs[publicinputs.FilledBaseAmount(slot)]
		g.AssertUnsigned(fill, 32)
		api.AssertIsEqual(fill, result.filledBaseAmount[slot])
	}

	g.AssertUnsigned(inputs[publicinputs.InternalMatchedBase], 35)
	g.AssertUnsigned(inputs[publicinputs.RequestedResidualBase], 35)
	g.AssertUnsigned(inputs[publicinputs.AMMResidualDirection], 2)
	g.AssertEnum(inputs[publicinputs.AMMResidualDirection], 0, 1, 2)
	g.AssertUnsigned(inputs[publicinputs.AMMFilledBase], 35)
	api.AssertIsEqual(
		inputs[publicinputs.InternalMatchedBase],
		result.internalMatchedBase,
	)
	api.AssertIsEqual(
		inputs[publicinputs.RequestedResidualBase],
		result.requestedResidualBase,
	)
	api.AssertIsEqual(
		inputs[publicinputs.AMMResidualDirection],
		result.ammResidualDirection,
	)
	api.AssertIsEqual(inputs[publicinputs.AMMFilledBase], result.ammFilledBase)
}
