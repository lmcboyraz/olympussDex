package engine

import (
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/cemilboyraz/oly2/internal/engine/amm"
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
)

func TestShowcaseBatch0ExactTransition(t *testing.T) {
	result, err := Execute(state.GenesisFixture(), ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	assertRootDecimal(
		t,
		result.OldStateRoot.String(),
		"7071633105518838241591574461399326375340121589417051538999520727165324367471",
	)
	assertRootDecimal(
		t,
		result.NewStateRoot.String(),
		"6086084270354442980753428173331985779337986395570386999756935893240657765167",
	)

	wantAccounts := [protocol.MaxSlots]state.Account{
		{BaseBalance: 150, QuoteBalance: 14_500},
		{BaseBalance: 150, QuoteBalance: 14_500},
		{BaseBalance: 150, QuoteBalance: 14_500},
		{BaseBalance: 120, QuoteBalance: 17_800},
		{BaseBalance: 80, QuoteBalance: 22_200},
		{BaseBalance: 80, QuoteBalance: 22_200},
		{BaseBalance: 80, QuoteBalance: 22_200},
		{BaseBalance: 80, QuoteBalance: 22_200},
	}
	if result.PostState.Accounts != wantAccounts {
		t.Fatalf("accounts = %#v, want %#v", result.PostState.Accounts, wantAccounts)
	}
	if result.PostState.AMM != (state.AMM{BaseReserve: 910, QuoteReserve: 109_900}) {
		t.Fatalf("AMM = %#v", result.PostState.AMM)
	}
	if result.PostState.Metadata != (state.Metadata{ProcessedBatchCount: 1, ProcessedMessageCount: 8}) {
		t.Fatalf("metadata = %#v", result.PostState.Metadata)
	}
	assertTrace(t, result, traceExpectation{
		demand:       200,
		supply:       80,
		matched:      80,
		requested:    120,
		direction:    amm.DirectionSellsBase,
		ammFilled:    90,
		unfilled:     30,
		fills:        [8]uint64{50, 50, 50, 20, 20, 20, 20, 20},
		statuses:     activeStatuses(),
		clearingTick: 109,
	})
}

func TestShowcaseBatch1ExactTransition(t *testing.T) {
	first, err := Execute(state.GenesisFixture(), ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	result, err := Execute(first.PostState, ShowcaseBatch1())
	if err != nil {
		t.Fatal(err)
	}
	assertRootDecimal(
		t,
		result.OldStateRoot.String(),
		"6086084270354442980753428173331985779337986395570386999756935893240657765167",
	)
	assertRootDecimal(
		t,
		result.NewStateRoot.String(),
		"6454644218358115299954897593039323931065349730735171915711406264296088160693",
	)

	wantAccounts := [protocol.MaxSlots]state.Account{
		{BaseBalance: 170, QuoteBalance: 12_300},
		{BaseBalance: 170, QuoteBalance: 12_300},
		{BaseBalance: 170, QuoteBalance: 12_300},
		{BaseBalance: 120, QuoteBalance: 17_800},
		{BaseBalance: 20, QuoteBalance: 28_800},
		{BaseBalance: 40, QuoteBalance: 26_600},
		{BaseBalance: 40, QuoteBalance: 26_600},
		{BaseBalance: 71, QuoteBalance: 23_190},
	}
	if result.PostState.Accounts != wantAccounts {
		t.Fatalf("accounts = %#v, want %#v", result.PostState.Accounts, wantAccounts)
	}
	if result.PostState.AMM != (state.AMM{BaseReserve: 999, QuoteReserve: 100_110}) {
		t.Fatalf("AMM = %#v", result.PostState.AMM)
	}
	if result.PostState.Metadata != (state.Metadata{ProcessedBatchCount: 2, ProcessedMessageCount: 16}) {
		t.Fatalf("metadata = %#v", result.PostState.Metadata)
	}
	statuses := activeStatuses()
	statuses[7] = protocol.FundingStatusRejectedUnfunded
	assertTrace(t, result, traceExpectation{
		demand:       60,
		supply:       180,
		matched:      60,
		requested:    120,
		direction:    amm.DirectionBuysBase,
		ammFilled:    89,
		unfilled:     31,
		fills:        [8]uint64{20, 20, 20, 60, 40, 40, 9, 0},
		statuses:     statuses,
		clearingTick: 109,
	})
}

func TestSequentialDepositAndReservation(t *testing.T) {
	tests := []struct {
		name         string
		slots        [protocol.MaxSlots]protocol.Message
		wantStatuses [protocol.MaxSlots]protocol.FundingStatus
	}{
		{
			name: "deposit before order funds it",
			slots: [protocol.MaxSlots]protocol.Message{
				depositMessage(0, 0, protocol.TokenQuote, 10),
				orderMessage(1, 0, protocol.SideBuy, 5, 1),
			},
			wantStatuses: [protocol.MaxSlots]protocol.FundingStatus{
				protocol.FundingStatusNotOrder,
				protocol.FundingStatusActive,
			},
		},
		{
			name: "order before deposit remains rejected",
			slots: [protocol.MaxSlots]protocol.Message{
				orderMessage(0, 0, protocol.SideBuy, 5, 1),
				depositMessage(1, 0, protocol.TokenQuote, 10),
			},
			wantStatuses: [protocol.MaxSlots]protocol.FundingStatus{
				protocol.FundingStatusRejectedUnfunded,
				protocol.FundingStatusNotOrder,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pre := state.GenesisFixture()
			pre.Accounts[0] = state.Account{}
			batch := batchWithSlots(0, 0, 2, test.slots)
			result, err := Execute(pre, batch)
			if err != nil {
				t.Fatal(err)
			}
			if result.FundingStatus != test.wantStatuses {
				t.Fatalf("statuses = %v, want %v", result.FundingStatus, test.wantStatuses)
			}
		})
	}
}

func TestUnderfundedBuyAndSellAndRepeatedReservations(t *testing.T) {
	pre := state.GenesisFixture()
	pre.Accounts[0] = state.Account{BaseBalance: 5, QuoteBalance: 10}
	slots := [protocol.MaxSlots]protocol.Message{
		orderMessage(0, 0, protocol.SideBuy, 5, 1),
		orderMessage(1, 0, protocol.SideBuy, 1, 1),
		orderMessage(2, 0, protocol.SideSell, 5, 1),
		orderMessage(3, 0, protocol.SideSell, 1, 1),
	}
	result, err := Execute(pre, batchWithSlots(0, 0, 4, slots))
	if err != nil {
		t.Fatal(err)
	}
	want := [protocol.MaxSlots]protocol.FundingStatus{
		protocol.FundingStatusActive,
		protocol.FundingStatusRejectedUnfunded,
		protocol.FundingStatusActive,
		protocol.FundingStatusRejectedUnfunded,
	}
	if result.FundingStatus != want {
		t.Fatalf("statuses = %v, want %v", result.FundingStatus, want)
	}
}

func TestSettlementProceedsCannotFundLaterReservation(t *testing.T) {
	pre := state.GenesisFixture()
	pre.Accounts[0] = state.Account{BaseBalance: 10}
	slots := [protocol.MaxSlots]protocol.Message{
		orderMessage(0, 0, protocol.SideSell, 10, 109),
		orderMessage(1, 1, protocol.SideBuy, 10, 109),
		orderMessage(2, 0, protocol.SideBuy, 1, 109),
	}
	result, err := Execute(pre, batchWithSlots(0, 0, 3, slots))
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingStatus != ([8]protocol.FundingStatus{
		protocol.FundingStatusActive,
		protocol.FundingStatusActive,
		protocol.FundingStatusRejectedUnfunded,
	}) {
		t.Fatalf("statuses = %v", result.FundingStatus)
	}
	if result.PostState.Accounts[0] != (state.Account{QuoteBalance: 1_100}) {
		t.Fatalf("account 0 = %#v", result.PostState.Accounts[0])
	}
}

func TestNoActiveOrders(t *testing.T) {
	slots := [protocol.MaxSlots]protocol.Message{
		depositMessage(0, 0, protocol.TokenBase, 1),
	}
	result, err := Execute(state.GenesisFixture(), batchWithSlots(0, 0, 1, slots))
	if err != nil {
		t.Fatal(err)
	}
	if result.ClearingTick != 0 ||
		result.InternalMatchedBase != 0 ||
		result.RequestedResidualBase != 0 ||
		result.AMMResidualDirection != amm.DirectionNone ||
		result.AMMFilledBase != 0 ||
		result.FilledBaseAmount != [8]uint64{} {
		t.Fatalf("non-zero empty trace: %#v", result)
	}
}

func TestOneSidedAndBalancedBatches(t *testing.T) {
	t.Run("one sided", func(t *testing.T) {
		slots := [protocol.MaxSlots]protocol.Message{
			orderMessage(0, 0, protocol.SideBuy, 10, 109),
		}
		result, err := Execute(state.GenesisFixture(), batchWithSlots(0, 0, 1, slots))
		if err != nil {
			t.Fatal(err)
		}
		if result.Demand != 10 || result.Supply != 0 ||
			result.InternalMatchedBase != 0 || result.AMMFilledBase != 10 ||
			result.FilledBaseAmount[0] != 10 {
			t.Fatalf("unexpected one-sided trace: %#v", result)
		}
	})

	t.Run("balanced", func(t *testing.T) {
		slots := [protocol.MaxSlots]protocol.Message{
			orderMessage(0, 0, protocol.SideBuy, 10, 99),
			orderMessage(1, 1, protocol.SideSell, 10, 99),
		}
		result, err := Execute(state.GenesisFixture(), batchWithSlots(0, 0, 2, slots))
		if err != nil {
			t.Fatal(err)
		}
		if result.InternalMatchedBase != 10 ||
			result.RequestedResidualBase != 0 ||
			result.AMMResidualDirection != amm.DirectionNone ||
			result.FilledBaseAmount != ([8]uint64{10, 10}) {
			t.Fatalf("unexpected balanced trace: %#v", result)
		}
	})
}

func TestStrictFIFOPartialBuyAndSell(t *testing.T) {
	tests := []struct {
		name      string
		preAMM    state.AMM
		slots     [protocol.MaxSlots]protocol.Message
		wantFills [protocol.MaxSlots]uint64
	}{
		{
			name:   "BUY FIFO",
			preAMM: state.AMM{BaseReserve: 10, QuoteReserve: 500},
			slots: [protocol.MaxSlots]protocol.Message{
				orderMessage(0, 0, protocol.SideBuy, 10, 99),
				orderMessage(1, 1, protocol.SideBuy, 10, 99),
				orderMessage(2, 2, protocol.SideSell, 10, 99),
			},
			wantFills: [protocol.MaxSlots]uint64{10, 5, 10},
		},
		{
			name:   "SELL FIFO",
			preAMM: state.AMM{BaseReserve: 100, QuoteReserve: 1_050},
			slots: [protocol.MaxSlots]protocol.Message{
				orderMessage(0, 0, protocol.SideSell, 10, 9),
				orderMessage(1, 1, protocol.SideSell, 10, 9),
				orderMessage(2, 2, protocol.SideBuy, 10, 9),
			},
			wantFills: [protocol.MaxSlots]uint64{10, 5, 10},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pre := state.GenesisFixture()
			pre.AMM = test.preAMM
			result, err := Execute(pre, batchWithSlots(0, 0, 3, test.slots))
			if err != nil {
				t.Fatal(err)
			}
			if result.FilledBaseAmount != test.wantFills {
				t.Fatalf("fills = %v, want %v", result.FilledBaseAmount, test.wantFills)
			}
		})
	}
}

func TestNonEligibleActiveOrderIsRefunded(t *testing.T) {
	pre := state.GenesisFixture()
	slots := [protocol.MaxSlots]protocol.Message{
		orderMessage(0, 0, protocol.SideBuy, 10, 99),
		orderMessage(1, 1, protocol.SideBuy, 1, 9),
		orderMessage(2, 2, protocol.SideSell, 10, 99),
	}
	result, err := Execute(pre, batchWithSlots(0, 0, 3, slots))
	if err != nil {
		t.Fatal(err)
	}
	if result.ClearingTick != 99 || result.FilledBaseAmount[1] != 0 {
		t.Fatalf("trace = %#v", result)
	}
	if result.PostState.Accounts[1] != pre.Accounts[1] {
		t.Fatalf("non-eligible account = %#v, want %#v", result.PostState.Accounts[1], pre.Accounts[1])
	}
}

func TestExecuteRejectsValidationMetadataAndArithmeticErrors(t *testing.T) {
	t.Run("protocol validation", func(t *testing.T) {
		_, err := Execute(state.GenesisFixture(), protocol.Batch{})
		assertErrorContains(t, err, "validate batch")
	})
	t.Run("batch index mismatch", func(t *testing.T) {
		batch := oneDepositBatch()
		batch.BatchIndex = 1
		_, err := Execute(state.GenesisFixture(), batch)
		assertErrorContains(t, err, "batch index")
	})
	t.Run("sequence mismatch", func(t *testing.T) {
		batch := oneDepositBatch()
		batch.StartSequenceID = 1
		batch.Slots[0].SequenceID = 1
		_, err := Execute(state.GenesisFixture(), batch)
		assertErrorContains(t, err, "start sequence")
	})
	t.Run("deposit overflow", func(t *testing.T) {
		pre := state.GenesisFixture()
		pre.Accounts[0].BaseBalance = math.MaxUint64
		_, err := Execute(pre, oneDepositBatch())
		assertErrorContains(t, err, "overflow")
	})
	t.Run("metadata message overflow", func(t *testing.T) {
		pre := state.GenesisFixture()
		pre.Metadata.ProcessedMessageCount = math.MaxUint64
		batch := oneDepositBatch()
		batch.StartSequenceID = math.MaxUint64
		batch.Slots[0].SequenceID = math.MaxUint64
		_, err := Execute(pre, batch)
		assertErrorContains(t, err, "processed message count")
	})
	t.Run("settlement credit overflow", func(t *testing.T) {
		pre := state.GenesisFixture()
		pre.Accounts[0] = state.Account{
			BaseBalance:  math.MaxUint64,
			QuoteBalance: 1,
		}
		pre.Accounts[1] = state.Account{BaseBalance: 1}
		slots := [protocol.MaxSlots]protocol.Message{
			orderMessage(0, 0, protocol.SideBuy, 1, 0),
			orderMessage(1, 1, protocol.SideSell, 1, 0),
		}
		_, err := Execute(pre, batchWithSlots(0, 0, 2, slots))
		assertErrorContains(t, err, "BUY base credit")
	})
}

func TestConservationDetectsFailure(t *testing.T) {
	pre := state.GenesisFixture()
	post := pre
	post.Accounts[0].BaseBalance++
	err := CheckConservation(pre, post, DepositTotals{})
	assertErrorContains(t, err, "BASE conservation")
}

func TestExecuteIsDeterministic(t *testing.T) {
	first, err := Execute(state.GenesisFixture(), ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Execute(state.GenesisFixture(), ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same input produced different results")
	}
}

func TestPublicInputsUseCanonicalOrder(t *testing.T) {
	result, err := Execute(state.GenesisFixture(), ShowcaseBatch0())
	if err != nil {
		t.Fatal(err)
	}
	inputs := result.PublicInputs()
	if len(inputs) != publicinputs.Count {
		t.Fatalf("input count = %d", len(inputs))
	}

	var oldRoot, newRoot big.Int
	result.OldStateRoot.BigInt(&oldRoot)
	result.NewStateRoot.BigInt(&newRoot)
	want := [publicinputs.Count]*big.Int{}
	want[publicinputs.BatchIndex] = new(big.Int).SetUint64(0)
	want[publicinputs.MessageCount] = new(big.Int).SetUint64(8)
	want[publicinputs.OldStateRoot] = &oldRoot
	want[publicinputs.BatchCommitmentHi] = new(big.Int).SetBytes(result.BatchCommitment.Hi[:])
	want[publicinputs.BatchCommitmentLo] = new(big.Int).SetBytes(result.BatchCommitment.Lo[:])
	want[publicinputs.NewStateRoot] = &newRoot
	want[publicinputs.ClearingTick] = new(big.Int).SetUint64(109)
	for index := 0; index < protocol.MaxSlots; index++ {
		want[publicinputs.FundingStatus(index)] = new(big.Int).SetUint64(uint64(protocol.FundingStatusActive))
		want[publicinputs.FilledBaseAmount(index)] = new(big.Int).SetUint64(result.FilledBaseAmount[index])
	}
	want[publicinputs.InternalMatchedBase] = new(big.Int).SetUint64(80)
	want[publicinputs.RequestedResidualBase] = new(big.Int).SetUint64(120)
	want[publicinputs.AMMResidualDirection] = new(big.Int).SetUint64(uint64(amm.DirectionSellsBase))
	want[publicinputs.AMMFilledBase] = new(big.Int).SetUint64(90)

	for index := range want {
		if inputs[index].Cmp(want[index]) != 0 {
			t.Fatalf("input %d (%s) = %s, want %s", index, publicinputs.Names()[index], inputs[index], want[index])
		}
	}
}

func TestPublicInputsRetainMessageCountForNonOrders(t *testing.T) {
	result, err := Execute(state.GenesisFixture(), oneDepositBatch())
	if err != nil {
		t.Fatal(err)
	}
	inputs := result.PublicInputs()
	if inputs[publicinputs.MessageCount].Uint64() != 1 {
		t.Fatalf(
			"messageCount = %s, want 1",
			inputs[publicinputs.MessageCount],
		)
	}
	if inputs[publicinputs.FundingStatus(0)].Uint64() != uint64(protocol.FundingStatusNotOrder) {
		t.Fatalf("deposit funding status = %s", inputs[publicinputs.FundingStatus(0)])
	}
}

type traceExpectation struct {
	demand       uint64
	supply       uint64
	matched      uint64
	requested    uint64
	direction    amm.Direction
	ammFilled    uint64
	unfilled     uint64
	fills        [protocol.MaxSlots]uint64
	statuses     [protocol.MaxSlots]protocol.FundingStatus
	clearingTick uint8
}

func assertTrace(t *testing.T, result Result, want traceExpectation) {
	t.Helper()
	if result.Demand != want.demand ||
		result.Supply != want.supply ||
		result.InternalMatchedBase != want.matched ||
		result.RequestedResidualBase != want.requested ||
		result.AMMResidualDirection != want.direction ||
		result.AMMFilledBase != want.ammFilled ||
		result.UnfilledResidualBase != want.unfilled ||
		result.FilledBaseAmount != want.fills ||
		result.FundingStatus != want.statuses ||
		result.ClearingTick != want.clearingTick {
		t.Fatalf("trace = %#v, want %#v", result, want)
	}
}

func activeStatuses() [protocol.MaxSlots]protocol.FundingStatus {
	var statuses [protocol.MaxSlots]protocol.FundingStatus
	for index := range statuses {
		statuses[index] = protocol.FundingStatusActive
	}
	return statuses
}

func oneDepositBatch() protocol.Batch {
	slots := [protocol.MaxSlots]protocol.Message{
		depositMessage(0, 0, protocol.TokenBase, 1),
	}
	return batchWithSlots(0, 0, 1, slots)
}

func batchWithSlots(
	batchIndex uint64,
	startSequenceID uint64,
	messageCount uint8,
	slots [protocol.MaxSlots]protocol.Message,
) protocol.Batch {
	return protocol.Batch{
		BatchIndex:      batchIndex,
		StartSequenceID: startSequenceID,
		MessageCount:    messageCount,
		Slots:           slots,
	}
}

func depositMessage(
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

func assertErrorContains(t *testing.T, err error, text string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), text) {
		t.Fatalf("error = %v, want containing %q", err, text)
	}
}

func assertRootDecimal(t *testing.T, got string, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("root = %s, want %s", got, want)
	}
}
