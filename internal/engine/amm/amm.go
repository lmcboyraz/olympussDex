// Package amm implements overflow-safe, fee-free constant-product residual
// execution for the deterministic reference engine.
package amm

import (
	"fmt"
	"math"
	"math/bits"
)

type Direction uint8

const (
	DirectionNone Direction = iota
	DirectionSellsBase
	DirectionBuysBase
)

type Reserves struct {
	Base  uint64
	Quote uint64
}

func (direction Direction) String() string {
	switch direction {
	case DirectionNone:
		return "NONE"
	case DirectionSellsBase:
		return "AMM_SELLS_BASE"
	case DirectionBuysBase:
		return "AMM_BUYS_BASE"
	default:
		return "UNKNOWN"
	}
}

// MaximumResidual finds the largest valid integer residual with a logarithmic
// binary search. For either direction, valid residuals form a prefix beginning
// at zero under the fee-free constant-product rule.
func MaximumResidual(
	pre Reserves,
	price uint64,
	requested uint64,
	direction Direction,
) (uint64, error) {
	if price == 0 {
		return 0, fmt.Errorf("price must be positive")
	}
	if direction != DirectionSellsBase && direction != DirectionBuysBase {
		return 0, fmt.Errorf("invalid residual direction %d", direction)
	}

	high := requested
	switch direction {
	case DirectionSellsBase:
		high = min(high, pre.Base)
		high = min(high, (math.MaxUint64-pre.Quote)/price)
	case DirectionBuysBase:
		high = min(high, math.MaxUint64-pre.Base)
		high = min(high, pre.Quote/price)
	}

	var low uint64
	for low < high {
		delta := high - low
		mid := low + delta/2
		if delta&1 == 1 {
			mid++
		}
		valid, err := ValidResidual(pre, price, mid, direction)
		if err != nil {
			return 0, err
		}
		if valid {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low, nil
}

func ValidResidual(
	pre Reserves,
	price uint64,
	residual uint64,
	direction Direction,
) (bool, error) {
	if price == 0 {
		return false, fmt.Errorf("price must be positive")
	}
	if direction != DirectionSellsBase && direction != DirectionBuysBase {
		return false, fmt.Errorf("invalid residual direction %d", direction)
	}
	post, err := Apply(pre, price, residual, direction)
	if err != nil {
		return false, nil
	}
	return productAtLeast(post, pre), nil
}

func Apply(
	pre Reserves,
	price uint64,
	residual uint64,
	direction Direction,
) (Reserves, error) {
	if price == 0 {
		return Reserves{}, fmt.Errorf("price must be positive")
	}
	quoteAmountHigh, quoteAmount := bits.Mul64(residual, price)
	if quoteAmountHigh != 0 {
		return Reserves{}, fmt.Errorf("residual quote amount overflows uint64")
	}

	switch direction {
	case DirectionSellsBase:
		if residual > pre.Base {
			return Reserves{}, fmt.Errorf("AMM base reserve underflow")
		}
		if quoteAmount > math.MaxUint64-pre.Quote {
			return Reserves{}, fmt.Errorf("AMM quote reserve overflow")
		}
		return Reserves{
			Base:  pre.Base - residual,
			Quote: pre.Quote + quoteAmount,
		}, nil
	case DirectionBuysBase:
		if residual > math.MaxUint64-pre.Base {
			return Reserves{}, fmt.Errorf("AMM base reserve overflow")
		}
		if quoteAmount > pre.Quote {
			return Reserves{}, fmt.Errorf("AMM quote reserve underflow")
		}
		return Reserves{
			Base:  pre.Base + residual,
			Quote: pre.Quote - quoteAmount,
		}, nil
	default:
		return Reserves{}, fmt.Errorf("invalid residual direction %d", direction)
	}
}

func productAtLeast(left Reserves, right Reserves) bool {
	leftHigh, leftLow := bits.Mul64(left.Base, left.Quote)
	rightHigh, rightLow := bits.Mul64(right.Base, right.Quote)
	if leftHigh != rightHigh {
		return leftHigh > rightHigh
	}
	return leftLow >= rightLow
}
