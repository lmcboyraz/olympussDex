package circuit

import (
	"fmt"
	"math"
	"math/big"

	"github.com/cemilboyraz/oly2/internal/engine/amm"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
)

type constrainedAMMResult struct {
	filled    frontend.Variable
	postBase  frontend.Variable
	postQuote frontend.Variable
}

func init() {
	solver.RegisterHint(maximumResidualHint)
}

func constrainMaximumResidual(
	api frontend.API,
	pre AMMWitness,
	price frontend.Variable,
	requested frontend.Variable,
	direction frontend.Variable,
) (constrainedAMMResult, error) {
	g := NewGadgets(api)
	hint, err := api.Compiler().NewHint(
		maximumResidualHint,
		1,
		pre.BaseReserve,
		pre.QuoteReserve,
		price,
		requested,
		direction,
	)
	if err != nil {
		return constrainedAMMResult{}, fmt.Errorf("maximum residual hint: %w", err)
	}
	filled := hint[0]
	g.AssertUnsigned(filled, 35)
	api.AssertIsEqual(g.IsLessOrEqual(filled, requested, 35), 1)

	directionSellsBase := g.IsEqual(direction, uint64(amm.DirectionSellsBase))
	directionBuysBase := g.IsEqual(direction, uint64(amm.DirectionBuysBase))
	directionActive := api.Add(directionSellsBase, directionBuysBase)
	g.AssertBoolean(directionActive)

	quoteAmount := g.MulBounded(filled, 35, price, 9, 44)
	uint64Maximum := frontend.Variable(uint64(math.MaxUint64))
	baseCapacity := api.Sub(uint64Maximum, pre.BaseReserve)
	quoteCapacity := api.Sub(uint64Maximum, pre.QuoteReserve)
	g.AssertUnsigned(baseCapacity, 64)
	g.AssertUnsigned(quoteCapacity, 64)

	filledWithinBaseReserve := g.IsLessOrEqual(filled, pre.BaseReserve, 64)
	quoteWithinQuoteCapacity := g.IsLessOrEqual(quoteAmount, quoteCapacity, 64)
	filledWithinBaseCapacity := g.IsLessOrEqual(filled, baseCapacity, 64)
	quoteWithinQuoteReserve := g.IsLessOrEqual(quoteAmount, pre.QuoteReserve, 64)
	g.AssertEqualWhen(directionSellsBase, filledWithinBaseReserve, 1)
	g.AssertEqualWhen(directionSellsBase, quoteWithinQuoteCapacity, 1)
	g.AssertEqualWhen(directionBuysBase, filledWithinBaseCapacity, 1)
	g.AssertEqualWhen(directionBuysBase, quoteWithinQuoteReserve, 1)

	postBase := g.SubNoUnderflow(
		pre.BaseReserve,
		api.Mul(directionSellsBase, filled),
		64,
	)
	postBase = g.AddNoOverflow(
		postBase,
		api.Mul(directionBuysBase, filled),
		64,
	)
	postQuote := g.AddNoOverflow(
		pre.QuoteReserve,
		api.Mul(directionSellsBase, quoteAmount),
		64,
	)
	postQuote = g.SubNoUnderflow(
		postQuote,
		api.Mul(directionBuysBase, quoteAmount),
		64,
	)

	preProduct := g.Mul64To128(pre.BaseReserve, pre.QuoteReserve)
	postProduct := g.Mul64To128(postBase, postQuote)
	api.AssertIsEqual(g.IsLessOrEqual(preProduct, postProduct, 128), 1)

	notRequestedMaximum := g.IsLess(filled, requested, 35)
	next := api.Add(filled, 1)
	g.AssertUnsigned(next, 35)
	nextQuoteAmount := g.MulBounded(next, 35, price, 9, 44)

	nextSellArithmeticValid := api.Mul(
		g.IsLessOrEqual(next, pre.BaseReserve, 64),
		g.IsLessOrEqual(nextQuoteAmount, quoteCapacity, 64),
	)
	nextBuyArithmeticValid := api.Mul(
		g.IsLessOrEqual(next, baseCapacity, 64),
		g.IsLessOrEqual(nextQuoteAmount, pre.QuoteReserve, 64),
	)
	g.AssertBoolean(nextSellArithmeticValid)
	g.AssertBoolean(nextBuyArithmeticValid)
	nextArithmeticValid := api.Add(
		api.Mul(directionSellsBase, nextSellArithmeticValid),
		api.Mul(directionBuysBase, nextBuyArithmeticValid),
	)
	g.AssertBoolean(nextArithmeticValid)

	nextSell := api.Mul(directionSellsBase, nextSellArithmeticValid, next)
	nextBuy := api.Mul(directionBuysBase, nextBuyArithmeticValid, next)
	nextSellQuote := api.Mul(directionSellsBase, nextSellArithmeticValid, nextQuoteAmount)
	nextBuyQuote := api.Mul(directionBuysBase, nextBuyArithmeticValid, nextQuoteAmount)
	nextPostBase := g.SubNoUnderflow(pre.BaseReserve, nextSell, 64)
	nextPostBase = g.AddNoOverflow(nextPostBase, nextBuy, 64)
	nextPostQuote := g.AddNoOverflow(pre.QuoteReserve, nextSellQuote, 64)
	nextPostQuote = g.SubNoUnderflow(nextPostQuote, nextBuyQuote, 64)
	nextProduct := g.Mul64To128(nextPostBase, nextPostQuote)
	nextProductInvalid := g.IsLess(nextProduct, preProduct, 128)
	mustFailProduct := api.Mul(
		notRequestedMaximum,
		directionActive,
		nextArithmeticValid,
	)
	g.AssertEqualWhen(mustFailProduct, nextProductInvalid, 1)

	return constrainedAMMResult{
		filled:    filled,
		postBase:  postBase,
		postQuote: postQuote,
	}, nil
}

func maximumResidualHint(
	_ *big.Int,
	inputs []*big.Int,
	outputs []*big.Int,
) error {
	if len(inputs) != 5 || len(outputs) != 1 {
		return fmt.Errorf("maximum residual hint arity")
	}
	for index, input := range inputs {
		if !input.IsUint64() {
			return fmt.Errorf("maximum residual input %d exceeds uint64", index)
		}
	}
	direction := amm.Direction(inputs[4].Uint64())
	if direction == amm.DirectionNone {
		if inputs[3].Sign() != 0 {
			return fmt.Errorf("NONE direction has nonzero requested residual")
		}
		outputs[0].SetUint64(0)
		return nil
	}
	filled, err := amm.MaximumResidual(
		amm.Reserves{
			Base:  inputs[0].Uint64(),
			Quote: inputs[1].Uint64(),
		},
		inputs[2].Uint64(),
		inputs[3].Uint64(),
		direction,
	)
	if err != nil {
		return err
	}
	outputs[0].SetUint64(filled)
	return nil
}
