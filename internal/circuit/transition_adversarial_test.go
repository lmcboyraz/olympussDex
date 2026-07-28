package circuit

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/cemilboyraz/oly2/internal/engine"
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/test"
)

func TestProductionCircuitEnforcesGlobalLiabilityCaps(t *testing.T) {
	t.Run("exact cap accepted", func(t *testing.T) {
		pre := state.State{AMM: state.AMM{BaseReserve: math.MaxUint64 - 1}}
		assertEngineTransitionSolved(t, pre, depositBatch(0, 0, 0, protocol.TokenBase, 1))
	})

	tests := map[string]struct {
		pre   state.State
		batch protocol.Batch
	}{
		"pre-state aggregate above cap": {
			pre: state.State{
				Accounts: [8]state.Account{
					{BaseBalance: math.MaxUint64},
					{BaseBalance: 1},
				},
			},
			batch: depositBatch(0, 0, 0, protocol.TokenQuote, 1),
		},
		"batch deposits above cap": {
			batch: batchWithMessages(
				orderlessDeposit(0, 0, protocol.TokenBase, math.MaxUint64),
				orderlessDeposit(1, 1, protocol.TokenBase, 1),
			),
		},
		"pre plus deposit above cap": {
			pre:   state.State{AMM: state.AMM{BaseReserve: math.MaxUint64}},
			batch: depositBatch(0, 0, 0, protocol.TokenBase, 1),
		},
	}
	for name, fixture := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := engine.Execute(fixture.pre, fixture.batch); err == nil {
				t.Fatal("reference engine unexpectedly accepted cap violation")
			}
			assignment := bindingOnlyAssignment(t, fixture.pre, fixture.batch)
			if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err == nil {
				t.Fatal("circuit unexpectedly accepted cap violation")
			}
		})
	}
}

func TestProductionCircuitMakesUnderfundedOrdersEconomicNoOps(t *testing.T) {
	tests := []struct {
		name string
		side protocol.Side
		pre  state.State
	}{
		{
			name: "BUY",
			side: protocol.SideBuy,
			pre: state.State{
				AMM: state.AMM{BaseReserve: 10, QuoteReserve: 100},
			},
		},
		{
			name: "SELL",
			side: protocol.SideSell,
			pre: state.State{
				AMM: state.AMM{BaseReserve: 10, QuoteReserve: 100},
			},
		},
	}
	for _, fixture := range tests {
		t.Run(fixture.name, func(t *testing.T) {
			batch := batchWithMessages(orderMessage(0, 0, fixture.side, 1, 0))
			assignment, result, err := NewAssignment(fixture.pre, batch)
			if err != nil {
				t.Fatal(err)
			}
			if result.FundingStatus[0] != protocol.FundingStatusRejectedUnfunded ||
				result.FilledBaseAmount[0] != 0 ||
				result.PostState.Accounts != fixture.pre.Accounts ||
				result.PostState.AMM != fixture.pre.AMM {
				t.Fatalf("underfunded order changed economic state: %#v", result)
			}
			if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
				t.Fatalf("underfunded no-op: %v", err)
			}
		})
	}
}

func TestProductionCircuitConstrainsEveryRealOrderTieBreakLayer(t *testing.T) {
	tests := []struct {
		name     string
		pre      state.State
		batch    protocol.Batch
		wantTick uint8
	}{
		{
			name: "highest F",
			pre: fundedState(
				state.AMM{BaseReserve: 100, QuoteReserve: 100},
				[8]state.Account{
					{QuoteBalance: 1_000},
					{QuoteBalance: 1_000},
					{BaseBalance: 10},
				},
			),
			batch: batchWithMessages(
				orderMessage(0, 0, protocol.SideBuy, 10, 20),
				orderMessage(1, 1, protocol.SideBuy, 20, 10),
				orderMessage(2, 2, protocol.SideSell, 10, 20),
			),
			wantTick: 10,
		},
		{
			name: "highest M after F tie",
			pre: fundedState(
				state.AMM{BaseReserve: 1, QuoteReserve: 18},
				[8]state.Account{
					{QuoteBalance: 3},
					{QuoteBalance: 6},
					{BaseBalance: 2},
					{BaseBalance: 1},
				},
			),
			batch: batchWithMessages(
				orderMessage(0, 0, protocol.SideBuy, 1, 2),
				orderMessage(1, 1, protocol.SideBuy, 1, 5),
				orderMessage(2, 2, protocol.SideSell, 2, 2),
				orderMessage(3, 3, protocol.SideSell, 1, 5),
			),
			wantTick: 2,
		},
		{
			name: "closest spot after F and M tie",
			pre: fundedState(
				state.AMM{BaseReserve: 100, QuoteReserve: 1_700},
				[8]state.Account{
					{QuoteBalance: 210},
					{BaseBalance: 10},
				},
			),
			batch: batchWithMessages(
				orderMessage(0, 0, protocol.SideBuy, 10, 20),
				orderMessage(1, 1, protocol.SideSell, 10, 10),
			),
			wantTick: 20,
		},
		{
			name: "lowest tick after all ties",
			pre: fundedState(
				state.AMM{BaseReserve: 100, QuoteReserve: 1_600},
				[8]state.Account{
					{QuoteBalance: 210},
					{BaseBalance: 10},
				},
			),
			batch: batchWithMessages(
				orderMessage(0, 0, protocol.SideBuy, 10, 20),
				orderMessage(1, 1, protocol.SideSell, 10, 10),
			),
			wantTick: 10,
		},
	}
	for _, fixture := range tests {
		t.Run(fixture.name, func(t *testing.T) {
			assignment, result, err := NewAssignment(fixture.pre, fixture.batch)
			if err != nil {
				t.Fatal(err)
			}
			if result.ClearingTick != fixture.wantTick {
				t.Fatalf("clearing tick = %d, want %d", result.ClearingTick, fixture.wantTick)
			}
			if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
				t.Fatalf("tie-break fixture: %v", err)
			}
		})
	}
}

func TestProductionCircuitHandlesCriticalFullUint64AMMBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		pre   state.State
		batch protocol.Batch
	}{
		{
			name: "AMM sells from max base reserve at tick 255",
			pre: fundedState(
				state.AMM{BaseReserve: math.MaxUint64},
				[8]state.Account{{QuoteBalance: 256}},
			),
			batch: batchWithMessages(
				orderMessage(0, 0, protocol.SideBuy, 1, 255),
			),
		},
		{
			name: "AMM buys into zero base against max quote reserve",
			pre: fundedState(
				state.AMM{QuoteReserve: math.MaxUint64},
				[8]state.Account{{BaseBalance: 1}},
			),
			batch: batchWithMessages(
				orderMessage(0, 0, protocol.SideSell, 1, 0),
			),
		},
	}
	for _, fixture := range tests {
		t.Run(fixture.name, func(t *testing.T) {
			assertEngineTransitionSolved(t, fixture.pre, fixture.batch)
		})
	}
}

func TestProductionCircuitRejectsNativeFieldWraparoundWitnesses(t *testing.T) {
	assignment, _, err := NewAssignment(state.GenesisFixture(), engine.ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("private uint64", func(t *testing.T) {
		tampered := *assignment
		tampered.PreState.Accounts[0].BaseBalance = new(big.Int).Sub(
			ecc.BN254.ScalarField(),
			big.NewInt(1),
		)
		if err := test.IsSolved(&Circuit{}, &tampered, ecc.BN254.ScalarField()); err == nil {
			t.Fatal("field -1 unexpectedly represented a uint64 balance")
		}
	})
	t.Run("public uint32", func(t *testing.T) {
		tampered := *assignment
		tampered.Public.Inputs[publicinputs.FilledBaseAmount(0)] = new(big.Int).Sub(
			ecc.BN254.ScalarField(),
			big.NewInt(1),
		)
		if err := test.IsSolved(&Circuit{}, &tampered, ecc.BN254.ScalarField()); err == nil {
			t.Fatal("field -1 unexpectedly represented a uint32 fill")
		}
	})
}

func TestProductionCircuitBoundedDeterministicDifferential(t *testing.T) {
	random := rand.New(rand.NewPCG(0x4f4c5932, 0x4d494c4553544f4e))
	for caseIndex := 0; caseIndex < 12; caseIndex++ {
		pre := state.GenesisFixture()
		messageCount := 1 + random.IntN(protocol.MaxSlots)
		messages := make([]protocol.Message, messageCount)
		for slot := range messages {
			if random.IntN(3) == 0 {
				messages[slot] = orderlessDeposit(
					uint64(slot),
					uint64(random.IntN(protocol.MaxSlots)),
					protocol.TokenID(random.IntN(2)),
					uint64(1+random.IntN(100)),
				)
			} else {
				messages[slot] = orderMessage(
					uint64(slot),
					uint64(random.IntN(protocol.MaxSlots)),
					protocol.Side(random.IntN(2)),
					uint64(1+random.IntN(50)),
					uint64(random.IntN(256)),
				)
			}
		}
		batch := batchWithMessages(messages...)
		assignment, _, err := NewAssignment(pre, batch)
		if err != nil {
			t.Fatalf("case %d engine rejected generated canonical transition: %v", caseIndex, err)
		}
		if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
			t.Fatalf("case %d circuit disagreed with engine: %v", caseIndex, err)
		}
	}
}

func assertEngineTransitionSolved(
	t *testing.T,
	pre state.State,
	batch protocol.Batch,
) {
	t.Helper()
	assignment, _, err := NewAssignment(pre, batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("engine-accepted transition did not satisfy circuit: %v", err)
	}
}

func bindingOnlyAssignment(
	t *testing.T,
	pre state.State,
	batch protocol.Batch,
) *Circuit {
	t.Helper()
	assignment := &Circuit{PreState: AssignState(pre), Batch: AssignBatch(batch)}
	for index := range assignment.Public.Inputs {
		assignment.Public.Inputs[index] = new(big.Int)
	}
	assignment.Public.Inputs[publicinputs.BatchIndex] = new(big.Int).SetUint64(batch.BatchIndex)
	assignment.Public.Inputs[publicinputs.MessageCount] = new(big.Int).SetUint64(uint64(batch.MessageCount))
	leaves, err := pre.Leaves()
	if err != nil {
		t.Fatal(err)
	}
	tree, err := state.BuildTree(leaves[:])
	if err != nil {
		t.Fatal(err)
	}
	root := tree.Root()
	root.BigInt(assignment.Public.Inputs[publicinputs.OldStateRoot].(*big.Int))
	commitment, err := protocol.CommitBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	assignment.Public.Inputs[publicinputs.BatchCommitmentHi].(*big.Int).SetBytes(commitment.Hi[:])
	assignment.Public.Inputs[publicinputs.BatchCommitmentLo].(*big.Int).SetBytes(commitment.Lo[:])
	return assignment
}

func fundedState(amm state.AMM, accounts [8]state.Account) state.State {
	return state.State{Accounts: accounts, AMM: amm}
}

func batchWithMessages(messages ...protocol.Message) protocol.Batch {
	var slots [protocol.MaxSlots]protocol.Message
	copy(slots[:], messages)
	return protocol.Batch{
		MessageCount: uint8(len(messages)),
		Slots:        slots,
	}
}

func orderlessDeposit(
	sequenceID uint64,
	accountID uint64,
	token protocol.TokenID,
	amount uint64,
) protocol.Message {
	return protocol.Message{
		Type:          protocol.MessageTypeDeposit,
		SequenceID:    sequenceID,
		AccountID:     accountID,
		TokenID:       token,
		DepositAmount: amount,
	}
}

func orderMessage(
	sequenceID uint64,
	accountID uint64,
	side protocol.Side,
	baseAmount uint64,
	limitTick uint64,
) protocol.Message {
	return protocol.Message{
		Type:       protocol.MessageTypeOrder,
		SequenceID: sequenceID,
		AccountID:  accountID,
		Side:       side,
		BaseAmount: baseAmount,
		LimitTick:  limitTick,
	}
}
