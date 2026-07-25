package amm

import (
	"math"
	"testing"
)

func TestMaximumResidualMatchesShowcaseExcessBuy(t *testing.T) {
	pre := Reserves{Base: 1_000, Quote: 100_000}
	got, err := MaximumResidual(pre, 110, 120, DirectionSellsBase)
	if err != nil {
		t.Fatalf("maximum residual: %v", err)
	}
	if got != 90 {
		t.Fatalf("maximum residual = %d, want 90", got)
	}
	assertResidualMaximal(t, pre, 110, 120, DirectionSellsBase, got)

	post, err := Apply(pre, 110, got, DirectionSellsBase)
	if err != nil {
		t.Fatalf("apply residual: %v", err)
	}
	if post != (Reserves{Base: 910, Quote: 109_900}) {
		t.Fatalf("post reserves = %+v", post)
	}
}

func TestMaximumResidualMatchesShowcaseExcessSell(t *testing.T) {
	pre := Reserves{Base: 910, Quote: 109_900}
	got, err := MaximumResidual(pre, 110, 120, DirectionBuysBase)
	if err != nil {
		t.Fatalf("maximum residual: %v", err)
	}
	if got != 89 {
		t.Fatalf("maximum residual = %d, want 89", got)
	}
	assertResidualMaximal(t, pre, 110, 120, DirectionBuysBase, got)

	post, err := Apply(pre, 110, got, DirectionBuysBase)
	if err != nil {
		t.Fatalf("apply residual: %v", err)
	}
	if post != (Reserves{Base: 999, Quote: 100_110}) {
		t.Fatalf("post reserves = %+v", post)
	}
}

func TestMaximumResidualCanFillAllRequested(t *testing.T) {
	tests := map[string]struct {
		pre       Reserves
		price     uint64
		requested uint64
		direction Direction
	}{
		"AMM sells base": {
			pre:       Reserves{Base: 1_000, Quote: 100_000},
			price:     110,
			requested: 50,
			direction: DirectionSellsBase,
		},
		"AMM buys base": {
			pre:       Reserves{Base: 1_000, Quote: 100_000},
			price:     90,
			requested: 50,
			direction: DirectionBuysBase,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := MaximumResidual(test.pre, test.price, test.requested, test.direction)
			if err != nil {
				t.Fatalf("maximum residual: %v", err)
			}
			if got != test.requested {
				t.Fatalf("maximum residual = %d, want %d", got, test.requested)
			}
		})
	}
}

func TestMaximumResidualCanBeZero(t *testing.T) {
	pre := Reserves{Base: 1_000, Quote: 100_000}
	for _, direction := range []Direction{DirectionSellsBase, DirectionBuysBase} {
		got, err := MaximumResidual(pre, 100, 50, direction)
		if err != nil {
			t.Fatalf("direction %d: %v", direction, err)
		}
		if got != 0 {
			t.Fatalf("direction %d residual = %d, want 0", direction, got)
		}
	}
}

func TestMaximumResidualHandlesUint64BoundariesWithoutWrapping(t *testing.T) {
	tests := []struct {
		pre       Reserves
		price     uint64
		requested uint64
		direction Direction
	}{
		{
			pre:       Reserves{Base: math.MaxUint64, Quote: math.MaxUint64},
			price:     math.MaxUint64,
			requested: math.MaxUint64,
			direction: DirectionSellsBase,
		},
		{
			pre:       Reserves{Base: math.MaxUint64, Quote: math.MaxUint64},
			price:     math.MaxUint64,
			requested: math.MaxUint64,
			direction: DirectionBuysBase,
		},
	}
	for _, test := range tests {
		residual, err := MaximumResidual(test.pre, test.price, test.requested, test.direction)
		if err != nil {
			t.Fatalf("maximum residual: %v", err)
		}
		valid, err := ValidResidual(test.pre, test.price, residual, test.direction)
		if err != nil {
			t.Fatalf("validate result: %v", err)
		}
		if !valid {
			t.Fatalf("returned residual %d is invalid", residual)
		}
	}
}

func TestApplyRejectsReserveOverflowAndUnderflow(t *testing.T) {
	tests := map[string]struct {
		pre       Reserves
		price     uint64
		residual  uint64
		direction Direction
	}{
		"base underflow": {
			pre:       Reserves{Base: 1, Quote: 1},
			price:     1,
			residual:  2,
			direction: DirectionSellsBase,
		},
		"quote overflow": {
			pre:       Reserves{Base: 2, Quote: math.MaxUint64},
			price:     1,
			residual:  1,
			direction: DirectionSellsBase,
		},
		"base overflow": {
			pre:       Reserves{Base: math.MaxUint64, Quote: 2},
			price:     1,
			residual:  1,
			direction: DirectionBuysBase,
		},
		"quote underflow": {
			pre:       Reserves{Base: 1, Quote: 1},
			price:     2,
			residual:  1,
			direction: DirectionBuysBase,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Apply(test.pre, test.price, test.residual, test.direction); err == nil {
				t.Fatal("unsafe reserve update unexpectedly accepted")
			}
		})
	}
}

func TestResidualRejectsInvalidParameters(t *testing.T) {
	pre := Reserves{Base: 1, Quote: 1}
	if _, err := MaximumResidual(pre, 0, 1, DirectionSellsBase); err == nil {
		t.Fatal("zero price unexpectedly accepted")
	}
	if _, err := MaximumResidual(pre, 1, 1, DirectionNone); err == nil {
		t.Fatal("NONE direction unexpectedly accepted")
	}
}

func assertResidualMaximal(
	t *testing.T,
	pre Reserves,
	price uint64,
	requested uint64,
	direction Direction,
	residual uint64,
) {
	t.Helper()
	valid, err := ValidResidual(pre, price, residual, direction)
	if err != nil {
		t.Fatalf("validate residual: %v", err)
	}
	if !valid {
		t.Fatalf("residual %d is invalid", residual)
	}
	if residual < requested {
		validNext, err := ValidResidual(pre, price, residual+1, direction)
		if err != nil {
			t.Fatalf("validate residual+1: %v", err)
		}
		if validNext {
			t.Fatalf("residual+1 (%d) is also valid", residual+1)
		}
	}
}
