// Package clearing evaluates active-order price candidates and applies the
// FIFO-Clear deterministic winner ordering.
package clearing

import (
	"fmt"
	"math/bits"

	"github.com/cemilboyraz/oly2/internal/engine/amm"
	"github.com/cemilboyraz/oly2/internal/protocol"
)

type Order struct {
	Slot       int
	Side       protocol.Side
	BaseAmount uint64
	LimitTick  uint8
}

type Uint128 struct {
	High uint64
	Low  uint64
}

type Candidate struct {
	Tick              uint8
	Demand            uint64
	Supply            uint64
	InternalMatched   uint64
	RequestedResidual uint64
	Direction         amm.Direction
	AMMFilled         uint64
	UnfilledResidual  uint64
	ExecutableBase    uint64
	SpotDistance      Uint128
}

type Result struct {
	ClearingTick      uint8
	Demand            uint64
	Supply            uint64
	InternalMatched   uint64
	RequestedResidual uint64
	Direction         amm.Direction
	AMMFilled         uint64
	UnfilledResidual  uint64
}

func Evaluate(orders []Order, preAMM amm.Reserves) (Result, error) {
	candidates, err := EvaluateCandidates(orders, preAMM)
	if err != nil {
		return Result{}, err
	}
	winner, ok := ChooseCandidate(candidates)
	if !ok {
		return Result{}, nil
	}
	return Result{
		ClearingTick:      winner.Tick,
		Demand:            winner.Demand,
		Supply:            winner.Supply,
		InternalMatched:   winner.InternalMatched,
		RequestedResidual: winner.RequestedResidual,
		Direction:         winner.Direction,
		AMMFilled:         winner.AMMFilled,
		UnfilledResidual:  winner.UnfilledResidual,
	}, nil
}

func EvaluateCandidates(
	orders []Order,
	preAMM amm.Reserves,
) ([]Candidate, error) {
	if len(orders) == 0 {
		return nil, nil
	}

	var ticks [256]bool
	for index, order := range orders {
		if order.Side != protocol.SideBuy && order.Side != protocol.SideSell {
			return nil, fmt.Errorf("order %d has invalid side %d", index, order.Side)
		}
		if order.BaseAmount == 0 {
			return nil, fmt.Errorf("order %d has zero base amount", index)
		}
		ticks[order.LimitTick] = true
	}

	candidates := make([]Candidate, 0, len(orders))
	for tick, present := range ticks {
		if !present {
			continue
		}
		candidate, err := evaluateTick(orders, preAMM, uint8(tick))
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func ChooseCandidate(candidates []Candidate) (Candidate, bool) {
	if len(candidates) == 0 {
		return Candidate{}, false
	}
	winner := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidateBetterThan(candidate, winner) {
			winner = candidate
		}
	}
	return winner, true
}

func SpotDistance(price uint64, baseReserve uint64, quoteReserve uint64) Uint128 {
	productHigh, productLow := bits.Mul64(price, baseReserve)
	if productHigh == 0 && productLow < quoteReserve {
		difference, _ := bits.Sub64(quoteReserve, productLow, 0)
		return Uint128{Low: difference}
	}
	differenceLow, borrow := bits.Sub64(productLow, quoteReserve, 0)
	differenceHigh, _ := bits.Sub64(productHigh, 0, borrow)
	return Uint128{High: differenceHigh, Low: differenceLow}
}

func evaluateTick(
	orders []Order,
	preAMM amm.Reserves,
	tick uint8,
) (Candidate, error) {
	var demand, supply uint64
	for _, order := range orders {
		switch order.Side {
		case protocol.SideBuy:
			if order.LimitTick >= tick {
				var err error
				demand, err = checkedAdd(demand, order.BaseAmount)
				if err != nil {
					return Candidate{}, fmt.Errorf("candidate %d demand: %w", tick, err)
				}
			}
		case protocol.SideSell:
			if order.LimitTick <= tick {
				var err error
				supply, err = checkedAdd(supply, order.BaseAmount)
				if err != nil {
					return Candidate{}, fmt.Errorf("candidate %d supply: %w", tick, err)
				}
			}
		}
	}

	internalMatched := min(demand, supply)
	var requested uint64
	direction := amm.DirectionNone
	switch {
	case demand > supply:
		requested = demand - supply
		direction = amm.DirectionSellsBase
	case supply > demand:
		requested = supply - demand
		direction = amm.DirectionBuysBase
	}

	var ammFilled uint64
	var err error
	if direction != amm.DirectionNone {
		ammFilled, err = amm.MaximumResidual(
			preAMM,
			uint64(tick)+1,
			requested,
			direction,
		)
		if err != nil {
			return Candidate{}, fmt.Errorf("candidate %d AMM residual: %w", tick, err)
		}
	}

	internalHigh, doubledInternal := bits.Mul64(internalMatched, 2)
	if internalHigh != 0 {
		return Candidate{}, fmt.Errorf("candidate %d internal fill overflow", tick)
	}
	executable, carry := bits.Add64(doubledInternal, ammFilled, 0)
	if carry != 0 {
		return Candidate{}, fmt.Errorf("candidate %d executable fill overflow", tick)
	}

	return Candidate{
		Tick:              tick,
		Demand:            demand,
		Supply:            supply,
		InternalMatched:   internalMatched,
		RequestedResidual: requested,
		Direction:         direction,
		AMMFilled:         ammFilled,
		UnfilledResidual:  requested - ammFilled,
		ExecutableBase:    executable,
		SpotDistance: SpotDistance(
			uint64(tick)+1,
			preAMM.Base,
			preAMM.Quote,
		),
	}, nil
}

func candidateBetterThan(candidate Candidate, incumbent Candidate) bool {
	if candidate.ExecutableBase != incumbent.ExecutableBase {
		return candidate.ExecutableBase > incumbent.ExecutableBase
	}
	if candidate.InternalMatched != incumbent.InternalMatched {
		return candidate.InternalMatched > incumbent.InternalMatched
	}
	if candidate.SpotDistance != incumbent.SpotDistance {
		return candidate.SpotDistance.less(incumbent.SpotDistance)
	}
	return candidate.Tick < incumbent.Tick
}

func (value Uint128) less(other Uint128) bool {
	if value.High != other.High {
		return value.High < other.High
	}
	return value.Low < other.Low
}

func checkedAdd(left uint64, right uint64) (uint64, error) {
	sum, carry := bits.Add64(left, right, 0)
	if carry != 0 {
		return 0, fmt.Errorf("uint64 overflow")
	}
	return sum, nil
}
