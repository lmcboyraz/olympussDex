package circuit

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
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
	if hex.EncodeToString(result.BatchCommitment.Digest[:]) !=
		"829d4f636e384c907cc28c6c3dc3567930d4aa370c8086f3843b0c1421889cfb" {
		t.Fatalf("batch commitment = %x", result.BatchCommitment.Digest)
	}
	if result.PostState.Accounts != ([8]state.Account{
		{BaseBalance: 150, QuoteBalance: 14_500},
		{BaseBalance: 150, QuoteBalance: 14_500},
		{BaseBalance: 150, QuoteBalance: 14_500},
		{BaseBalance: 120, QuoteBalance: 17_800},
		{BaseBalance: 80, QuoteBalance: 22_200},
		{BaseBalance: 80, QuoteBalance: 22_200},
		{BaseBalance: 80, QuoteBalance: 22_200},
		{BaseBalance: 80, QuoteBalance: 22_200},
	}) {
		t.Fatalf("tracked accounts changed: %#v", result.PostState.Accounts)
	}
	if result.PostState.AMM != (state.AMM{BaseReserve: 910, QuoteReserve: 109_900}) ||
		result.PostState.Metadata != (state.Metadata{
			ProcessedBatchCount:   1,
			ProcessedMessageCount: 8,
		}) {
		t.Fatalf("tracked post-state changed: %#v", result.PostState)
	}
	if result.FundingStatus != ([8]protocol.FundingStatus{2, 2, 2, 2, 2, 2, 2, 2}) ||
		result.ClearingTick != 109 ||
		result.Demand != 200 ||
		result.Supply != 80 ||
		result.InternalMatchedBase != 80 ||
		result.RequestedResidualBase != 120 ||
		result.AMMResidualDirection != 1 ||
		result.AMMFilledBase != 90 ||
		result.UnfilledResidualBase != 30 ||
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

func TestProductionCircuitBindsEveryPrivateInput(t *testing.T) {
	assignment, _, err := NewAssignment(state.GenesisFixture(), engine.ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	paths := append(
		collectPrivateWitnessPaths("PreState", reflect.ValueOf(assignment.PreState)),
		collectPrivateWitnessPaths("Batch", reflect.ValueOf(assignment.Batch))...,
	)
	const explicitPrivateInputs = 87
	if len(paths) != explicitPrivateInputs {
		t.Fatalf("private witness fields = %d, want %d", len(paths), explicitPrivateInputs)
	}
	for _, path := range paths {
		path := path
		t.Run(path.name, func(t *testing.T) {
			tampered := *assignment
			field := privateWitnessField(&tampered, path)
			changed := new(big.Int).Add(toBigInt(t, field.Interface()), big.NewInt(1))
			field.Set(reflect.ValueOf(changed))
			if err := test.IsSolved(&Circuit{}, &tampered, ecc.BN254.ScalarField()); err == nil {
				t.Fatalf("tampered private input %s unexpectedly satisfied", path.name)
			}
		})
	}
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
	if result.OldStateRoot.String() !=
		"6086084270354442980753428173331985779337986395570386999756935893240657765167" {
		t.Fatalf("old root = %s", result.OldStateRoot.String())
	}
	if result.NewStateRoot.String() !=
		"6454644218358115299954897593039323931065349730735171915711406264296088160693" {
		t.Fatalf("new root = %s", result.NewStateRoot.String())
	}
	if hex.EncodeToString(result.BatchCommitment.Digest[:]) !=
		"6465c2f57bbb2d904b86b8b882a0d4b346f24e6c4503ba9bc75e3254b4634d15" {
		t.Fatalf("batch commitment = %x", result.BatchCommitment.Digest)
	}
	if result.PostState.Accounts != ([8]state.Account{
		{BaseBalance: 170, QuoteBalance: 12_300},
		{BaseBalance: 170, QuoteBalance: 12_300},
		{BaseBalance: 170, QuoteBalance: 12_300},
		{BaseBalance: 120, QuoteBalance: 17_800},
		{BaseBalance: 20, QuoteBalance: 28_800},
		{BaseBalance: 40, QuoteBalance: 26_600},
		{BaseBalance: 40, QuoteBalance: 26_600},
		{BaseBalance: 71, QuoteBalance: 23_190},
	}) {
		t.Fatalf("tracked accounts changed: %#v", result.PostState.Accounts)
	}
	if result.PostState.AMM != (state.AMM{BaseReserve: 999, QuoteReserve: 100_110}) ||
		result.PostState.Metadata != (state.Metadata{
			ProcessedBatchCount:   2,
			ProcessedMessageCount: 16,
		}) {
		t.Fatalf("tracked post-state changed: %#v", result.PostState)
	}
	if result.FundingStatus != ([8]protocol.FundingStatus{2, 2, 2, 2, 2, 2, 2, 1}) ||
		result.ClearingTick != 109 ||
		result.Demand != 60 ||
		result.Supply != 180 ||
		result.InternalMatchedBase != 60 ||
		result.RequestedResidualBase != 120 ||
		result.AMMResidualDirection != 2 ||
		result.AMMFilledBase != 89 ||
		result.UnfilledResidualBase != 31 ||
		result.FilledBaseAmount != ([8]uint64{20, 20, 20, 60, 40, 40, 9, 0}) {
		t.Fatalf("tracked trace changed: %#v", result)
	}
	if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("valid production assignment: %v", err)
	}
}

type privateWitnessPath struct {
	root  string
	name  string
	steps []int
}

func collectPrivateWitnessPaths(root string, value reflect.Value) []privateWitnessPath {
	var paths []privateWitnessPath
	var walk func(reflect.Value, string, []int)
	walk = func(current reflect.Value, name string, steps []int) {
		switch current.Kind() {
		case reflect.Struct:
			for index := 0; index < current.NumField(); index++ {
				fieldName := current.Type().Field(index).Name
				nextSteps := append(append([]int(nil), steps...), index)
				walk(current.Field(index), name+"."+fieldName, nextSteps)
			}
		case reflect.Array:
			for index := 0; index < current.Len(); index++ {
				nextSteps := append(append([]int(nil), steps...), index)
				walk(current.Index(index), fmt.Sprintf("%s[%d]", name, index), nextSteps)
			}
		case reflect.Interface:
			paths = append(paths, privateWitnessPath{
				root:  root,
				name:  name,
				steps: append([]int(nil), steps...),
			})
		default:
			panic(fmt.Sprintf("unexpected private witness kind %s at %s", current.Kind(), name))
		}
	}
	walk(value, root, nil)
	return paths
}

func privateWitnessField(assignment *Circuit, path privateWitnessPath) reflect.Value {
	var value reflect.Value
	switch path.root {
	case "PreState":
		value = reflect.ValueOf(&assignment.PreState).Elem()
	case "Batch":
		value = reflect.ValueOf(&assignment.Batch).Elem()
	default:
		panic("unknown private witness root " + path.root)
	}
	for _, step := range path.steps {
		switch value.Kind() {
		case reflect.Struct:
			value = value.Field(step)
		case reflect.Array:
			value = value.Index(step)
		default:
			panic(fmt.Sprintf("unexpected private witness path kind %s", value.Kind()))
		}
	}
	return value
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
