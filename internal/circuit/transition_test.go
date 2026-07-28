package circuit

import (
	"math/big"
	"testing"

	"github.com/cemilboyraz/oly2/internal/engine"
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/test"
)

func TestProductionCircuitAcceptsTrackedShowcaseBatch0(t *testing.T) {
	assignment, result, err := NewAssignment(state.GenesisFixture(), engine.ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	if result.OldStateRoot.String() !=
		"7071633105518838241591574461399326375340121589417051538999520727165324367471" {
		t.Fatalf("old root = %s", result.OldStateRoot.String())
	}
	if result.NewStateRoot.String() !=
		"6086084270354442980753428173331985779337986395570386999756935893240657765167" {
		t.Fatalf("new root = %s", result.NewStateRoot.String())
	}
	if result.ClearingTick != 109 ||
		result.InternalMatchedBase != 80 ||
		result.RequestedResidualBase != 120 ||
		result.AMMFilledBase != 90 ||
		result.FilledBaseAmount != ([8]uint64{50, 50, 50, 20, 20, 20, 20, 20}) {
		t.Fatalf("tracked trace changed: %#v", result)
	}
	if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("valid production assignment: %v", err)
	}
}

func TestProductionCircuitBindsEveryPublicInput(t *testing.T) {
	assignment, _, err := NewAssignment(state.GenesisFixture(), engine.ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	for index, name := range publicinputs.Names() {
		t.Run(name, func(t *testing.T) {
			tampered := *assignment
			original := new(big.Int).Set(toBigInt(t, assignment.Public.Inputs[index]))
			tampered.Public.Inputs[index] = original.Add(original, big.NewInt(1))
			if err := test.IsSolved(&Circuit{}, &tampered, ecc.BN254.ScalarField()); err == nil {
				t.Fatalf("tampered public input %d (%s) unexpectedly satisfied", index, name)
			}
		})
	}
}

func TestProductionCircuitRejectsChangedPrivateInputs(t *testing.T) {
	assignment, _, err := NewAssignment(state.GenesisFixture(), engine.ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("pre-state", func(t *testing.T) {
		tampered := *assignment
		tampered.PreState.Accounts[0].BaseBalance = 101
		if err := test.IsSolved(&Circuit{}, &tampered, ecc.BN254.ScalarField()); err == nil {
			t.Fatal("tampered private pre-state unexpectedly satisfied")
		}
	})
	t.Run("batch", func(t *testing.T) {
		tampered := *assignment
		tampered.Batch.Slots[0].BaseAmount = 49
		if err := test.IsSolved(&Circuit{}, &tampered, ecc.BN254.ScalarField()); err == nil {
			t.Fatal("tampered private batch unexpectedly satisfied")
		}
	})
}

func TestProductionCircuitAcceptsTrackedShowcaseBatch1(t *testing.T) {
	first, err := engine.Execute(state.GenesisFixture(), engine.ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	assignment, result, err := NewAssignment(first.PostState, engine.ShowcaseBatch1())
	if err != nil {
		t.Fatal(err)
	}
	if result.NewStateRoot.String() !=
		"6454644218358115299954897593039323931065349730735171915711406264296088160693" {
		t.Fatalf("new root = %s", result.NewStateRoot.String())
	}
	if result.ClearingTick != 109 ||
		result.InternalMatchedBase != 60 ||
		result.RequestedResidualBase != 120 ||
		result.AMMFilledBase != 89 ||
		result.FilledBaseAmount != ([8]uint64{20, 20, 20, 60, 40, 40, 9, 0}) {
		t.Fatalf("tracked trace changed: %#v", result)
	}
	if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("valid production assignment: %v", err)
	}
}

func toBigInt(t *testing.T, value any) *big.Int {
	t.Helper()
	switch typed := value.(type) {
	case *big.Int:
		return typed
	case big.Int:
		return &typed
	case uint64:
		return new(big.Int).SetUint64(typed)
	case uint8:
		return new(big.Int).SetUint64(uint64(typed))
	case int:
		return big.NewInt(int64(typed))
	default:
		t.Fatalf("unsupported witness value %T", value)
		return nil
	}
}

func depositBatch(
	batchIndex uint64,
	startSequence uint64,
	account uint64,
	token protocol.TokenID,
	amount uint64,
) protocol.Batch {
	var slots [protocol.MaxSlots]protocol.Message
	slots[0] = protocol.Message{
		Type:          protocol.MessageTypeDeposit,
		SequenceID:    startSequence,
		AccountID:     account,
		TokenID:       token,
		DepositAmount: amount,
	}
	return protocol.Batch{
		BatchIndex:      batchIndex,
		StartSequenceID: startSequence,
		MessageCount:    1,
		Slots:           slots,
	}
}
