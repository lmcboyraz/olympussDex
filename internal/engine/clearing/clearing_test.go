package clearing

import (
	"testing"

	"github.com/cemilboyraz/oly2/internal/engine/amm"
	"github.com/cemilboyraz/oly2/internal/protocol"
)

func TestEvaluateCandidatesUsesOnlyEligibleActiveOrders(t *testing.T) {
	orders := []Order{
		{Slot: 0, Side: protocol.SideBuy, BaseAmount: 10, LimitTick: 20},
		{Slot: 1, Side: protocol.SideBuy, BaseAmount: 7, LimitTick: 10},
		{Slot: 2, Side: protocol.SideSell, BaseAmount: 5, LimitTick: 15},
		{Slot: 3, Side: protocol.SideSell, BaseAmount: 3, LimitTick: 20},
	}
	candidates, err := EvaluateCandidates(
		orders,
		amm.Reserves{Base: 0, Quote: 0},
	)
	if err != nil {
		t.Fatalf("evaluate candidates: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidate count = %d, want 3", len(candidates))
	}

	assertCandidateTotals(t, candidates[0], 10, 17, 0)
	assertCandidateTotals(t, candidates[1], 15, 10, 5)
	assertCandidateTotals(t, candidates[2], 20, 10, 8)
}

func TestEvaluateSelectsWinnerAcrossMultipleTicks(t *testing.T) {
	orders := []Order{
		{Slot: 0, Side: protocol.SideBuy, BaseAmount: 10, LimitTick: 20},
		{Slot: 1, Side: protocol.SideSell, BaseAmount: 10, LimitTick: 10},
	}
	result, err := Evaluate(
		orders,
		amm.Reserves{Base: 100, Quote: 1_600},
	)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// Both ticks execute 20 base with M=10 and are equally distant from the
	// pre-batch spot (prices 11 and 21 around spot 16); the lower tick wins.
	if result.ClearingTick != 10 {
		t.Fatalf("clearing tick = %d, want 10", result.ClearingTick)
	}
	if result.Demand != 10 || result.Supply != 10 || result.InternalMatched != 10 {
		t.Fatalf("result = %+v", result)
	}
}

func TestEvaluateRealOrdersPrefersClosestPreBatchSpotAfterFTies(t *testing.T) {
	orders := []Order{
		{Slot: 0, Side: protocol.SideBuy, BaseAmount: 10, LimitTick: 20},
		{Slot: 1, Side: protocol.SideSell, BaseAmount: 10, LimitTick: 10},
	}
	preAMM := amm.Reserves{Base: 100, Quote: 1_700}

	candidates, err := EvaluateCandidates(orders, preAMM)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].Tick != 10 ||
		candidates[0].ExecutableBase != 20 ||
		candidates[0].InternalMatched != 10 ||
		candidates[0].SpotDistance != (Uint128{Low: 600}) {
		t.Fatalf("tick 10 candidate = %+v", candidates[0])
	}
	if candidates[1].Tick != 20 ||
		candidates[1].ExecutableBase != 20 ||
		candidates[1].InternalMatched != 10 ||
		candidates[1].SpotDistance != (Uint128{Low: 400}) {
		t.Fatalf("tick 20 candidate = %+v", candidates[1])
	}

	result, err := Evaluate(orders, preAMM)
	if err != nil {
		t.Fatal(err)
	}
	if result.ClearingTick != 20 {
		t.Fatalf("clearing tick = %d, want spot-closer tick 20", result.ClearingTick)
	}
}

func TestEvaluateRealOrdersCarriesExecutableFillIntoFirstTieBreak(t *testing.T) {
	orders := []Order{
		{Slot: 0, Side: protocol.SideBuy, BaseAmount: 10, LimitTick: 20},
		{Slot: 1, Side: protocol.SideBuy, BaseAmount: 20, LimitTick: 10},
		{Slot: 2, Side: protocol.SideSell, BaseAmount: 10, LimitTick: 20},
	}
	preAMM := amm.Reserves{Base: 100, Quote: 100}

	candidates, err := EvaluateCandidates(orders, preAMM)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].Tick != 10 ||
		candidates[0].ExecutableBase != 30 ||
		candidates[0].InternalMatched != 0 {
		t.Fatalf("tick 10 candidate = %+v", candidates[0])
	}
	if candidates[1].Tick != 20 ||
		candidates[1].ExecutableBase != 20 ||
		candidates[1].InternalMatched != 10 {
		t.Fatalf("tick 20 candidate = %+v", candidates[1])
	}

	result, err := Evaluate(orders, preAMM)
	if err != nil {
		t.Fatal(err)
	}
	if result.ClearingTick != 10 {
		t.Fatalf(
			"clearing tick = %d, want higher-F tick 10 despite lower M",
			result.ClearingTick,
		)
	}
}

func TestChooseCandidateFirstPrefersHighestExecutableFill(t *testing.T) {
	winner, ok := ChooseCandidate([]Candidate{
		{Tick: 1, ExecutableBase: 9, InternalMatched: 8},
		{Tick: 2, ExecutableBase: 10, InternalMatched: 1},
	})
	if !ok || winner.Tick != 2 {
		t.Fatalf("winner = %+v, ok=%t", winner, ok)
	}
}

func TestChooseCandidateThenPrefersHighestInternalMatch(t *testing.T) {
	winner, ok := ChooseCandidate([]Candidate{
		{Tick: 1, ExecutableBase: 10, InternalMatched: 4},
		{Tick: 2, ExecutableBase: 10, InternalMatched: 5},
	})
	if !ok || winner.Tick != 2 {
		t.Fatalf("winner = %+v, ok=%t", winner, ok)
	}
}

func TestChooseCandidateThenPrefersClosestPreBatchSpot(t *testing.T) {
	winner, ok := ChooseCandidate([]Candidate{
		{
			Tick:            1,
			ExecutableBase:  10,
			InternalMatched: 5,
			SpotDistance:    Uint128{Low: 2},
		},
		{
			Tick:            2,
			ExecutableBase:  10,
			InternalMatched: 5,
			SpotDistance:    Uint128{Low: 1},
		},
	})
	if !ok || winner.Tick != 2 {
		t.Fatalf("winner = %+v, ok=%t", winner, ok)
	}
}

func TestChooseCandidateFinallyPrefersLowestTick(t *testing.T) {
	winner, ok := ChooseCandidate([]Candidate{
		{
			Tick:            2,
			ExecutableBase:  10,
			InternalMatched: 5,
			SpotDistance:    Uint128{Low: 1},
		},
		{
			Tick:            1,
			ExecutableBase:  10,
			InternalMatched: 5,
			SpotDistance:    Uint128{Low: 1},
		},
	})
	if !ok || winner.Tick != 1 {
		t.Fatalf("winner = %+v, ok=%t", winner, ok)
	}
}

func TestEvaluateWithoutActiveOrdersReturnsZeroTrace(t *testing.T) {
	result, err := Evaluate(nil, amm.Reserves{Base: 1_000, Quote: 100_000})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result != (Result{}) {
		t.Fatalf("result = %+v, want zero", result)
	}
}

func TestSpotDistanceUsesUnsigned128BitArithmetic(t *testing.T) {
	distance := SpotDistance(
		255,
		^uint64(0),
		^uint64(0),
	)
	if distance.High == 0 {
		t.Fatalf("distance did not retain high 64 bits: %+v", distance)
	}
}

func assertCandidateTotals(
	t *testing.T,
	candidate Candidate,
	tick uint8,
	demand uint64,
	supply uint64,
) {
	t.Helper()
	if candidate.Tick != tick ||
		candidate.Demand != demand ||
		candidate.Supply != supply {
		t.Fatalf(
			"candidate = %+v, want tick=%d demand=%d supply=%d",
			candidate,
			tick,
			demand,
			supply,
		)
	}
}
