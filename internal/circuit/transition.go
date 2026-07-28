package circuit

import (
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/consensys/gnark/frontend"
)

type constrainedOrder struct {
	active       frontend.Variable
	activeBuy    frontend.Variable
	activeSell   frontend.Variable
	accountID    frontend.Variable
	baseAmount   frontend.Variable
	limitTick    frontend.Variable
	buyReserved  frontend.Variable
	sellReserved frontend.Variable
}

type clearingCandidate struct {
	valid             frontend.Variable
	tick              frontend.Variable
	demand            frontend.Variable
	supply            frontend.Variable
	internalMatched   frontend.Variable
	requestedResidual frontend.Variable
	direction         frontend.Variable
	ammFilled         frontend.Variable
	executableBase    frontend.Variable
	spotDistance      frontend.Variable
	postBaseReserve   frontend.Variable
	postQuoteReserve  frontend.Variable
}

type transitionResult struct {
	postState             StateWitness
	fundingStatus         [protocol.MaxSlots]frontend.Variable
	filledBaseAmount      [protocol.MaxSlots]frontend.Variable
	clearingTick          frontend.Variable
	internalMatchedBase   frontend.Variable
	requestedResidualBase frontend.Variable
	ammResidualDirection  frontend.Variable
	ammFilledBase         frontend.Variable
}

func constrainTransition(
	api frontend.API,
	pre StateWitness,
	batch BatchWitness,
	flags [protocol.MaxSlots]messageFlags,
) (transitionResult, error) {
	g := NewGadgets(api)
	api.AssertIsEqual(pre.Metadata.ProcessedBatchCount, batch.BatchIndex)
	api.AssertIsEqual(pre.Metadata.ProcessedMessageCount, batch.StartSequenceID)

	preBaseLiability, preQuoteLiability := constrainLiabilities(api, pre)
	depositBase, depositQuote := constrainDepositTotals(api, batch, flags)
	expectedBase := g.AddNoOverflow(preBaseLiability, depositBase, 64)
	expectedQuote := g.AddNoOverflow(preQuoteLiability, depositQuote, 64)

	working := pre
	var orders [protocol.MaxSlots]constrainedOrder
	var result transitionResult
	for slot, message := range batch.Slots {
		depositBaseFlag := flags[slot].baseToken
		depositQuoteFlag := api.Mul(flags[slot].deposit, message.TokenID)
		for account := range working.Accounts {
			target := g.IsEqual(message.AccountID, account)
			baseDelta := api.Mul(depositBaseFlag, target, message.DepositAmount)
			quoteDelta := api.Mul(depositQuoteFlag, target, message.DepositAmount)
			working.Accounts[account].BaseBalance = g.AddNoOverflow(
				working.Accounts[account].BaseBalance,
				baseDelta,
				64,
			)
			working.Accounts[account].QuoteBalance = g.AddNoOverflow(
				working.Accounts[account].QuoteBalance,
				quoteDelta,
				64,
			)
		}

		accountBase := selectAccountBalance(api, working.Accounts, message.AccountID, false)
		accountQuote := selectAccountBalance(api, working.Accounts, message.AccountID, true)
		limitPrice := api.Add(message.LimitTick, 1)
		g.AssertUnsigned(limitPrice, 9)
		requiredQuote := g.MulBounded(message.BaseAmount, 32, limitPrice, 9, 41)

		buyFunded := api.Sub(1, g.IsLess(accountQuote, requiredQuote, 64))
		sellFunded := api.Sub(1, g.IsLess(accountBase, message.BaseAmount, 64))
		activeBuy := api.Mul(flags[slot].buy, buyFunded)
		activeSell := api.Mul(flags[slot].sell, sellFunded)
		active := api.Add(activeBuy, activeSell)
		g.AssertBoolean(activeBuy)
		g.AssertBoolean(activeSell)
		g.AssertBoolean(active)

		result.fundingStatus[slot] = api.Add(flags[slot].order, active)
		g.AssertUnsigned(result.fundingStatus[slot], 2)

		buyReserved := api.Mul(activeBuy, requiredQuote)
		sellReserved := api.Mul(activeSell, message.BaseAmount)
		for account := range working.Accounts {
			target := g.IsEqual(message.AccountID, account)
			working.Accounts[account].BaseBalance = g.SubNoUnderflow(
				working.Accounts[account].BaseBalance,
				api.Mul(target, sellReserved),
				64,
			)
			working.Accounts[account].QuoteBalance = g.SubNoUnderflow(
				working.Accounts[account].QuoteBalance,
				api.Mul(target, buyReserved),
				64,
			)
		}
		orders[slot] = constrainedOrder{
			active:       active,
			activeBuy:    activeBuy,
			activeSell:   activeSell,
			accountID:    message.AccountID,
			baseAmount:   message.BaseAmount,
			limitTick:    message.LimitTick,
			buyReserved:  buyReserved,
			sellReserved: sellReserved,
		}
	}

	winner, err := constrainClearing(api, orders, pre.AMM)
	if err != nil {
		return transitionResult{}, err
	}
	result.clearingTick = winner.tick
	result.internalMatchedBase = winner.internalMatched
	result.requestedResidualBase = winner.requestedResidual
	result.ammResidualDirection = winner.direction
	result.ammFilledBase = winner.ammFilled

	fills := constrainFIFO(api, orders, winner)
	result.filledBaseAmount = fills
	working.Accounts = constrainSettlement(api, working.Accounts, orders, fills, winner.tick)
	working.AMM.BaseReserve = winner.postBaseReserve
	working.AMM.QuoteReserve = winner.postQuoteReserve
	working.Metadata.ProcessedBatchCount = g.AddNoOverflow(
		pre.Metadata.ProcessedBatchCount,
		1,
		64,
	)
	working.Metadata.ProcessedMessageCount = g.AddNoOverflow(
		pre.Metadata.ProcessedMessageCount,
		batch.MessageCount,
		64,
	)

	postBaseLiability, postQuoteLiability := constrainLiabilities(api, working)
	api.AssertIsEqual(postBaseLiability, expectedBase)
	api.AssertIsEqual(postQuoteLiability, expectedQuote)
	result.postState = working
	return result, nil
}

func constrainLiabilities(
	api frontend.API,
	value StateWitness,
) (frontend.Variable, frontend.Variable) {
	g := NewGadgets(api)
	base := frontend.Variable(0)
	quote := frontend.Variable(0)
	for _, account := range value.Accounts {
		base = g.AddNoOverflow(base, account.BaseBalance, 64)
		quote = g.AddNoOverflow(quote, account.QuoteBalance, 64)
	}
	base = g.AddNoOverflow(base, value.AMM.BaseReserve, 64)
	quote = g.AddNoOverflow(quote, value.AMM.QuoteReserve, 64)
	return base, quote
}

func constrainDepositTotals(
	api frontend.API,
	batch BatchWitness,
	flags [protocol.MaxSlots]messageFlags,
) (frontend.Variable, frontend.Variable) {
	g := NewGadgets(api)
	base := frontend.Variable(0)
	quote := frontend.Variable(0)
	for slot, message := range batch.Slots {
		base = g.AddNoOverflow(
			base,
			api.Mul(flags[slot].baseToken, message.DepositAmount),
			64,
		)
		quote = g.AddNoOverflow(
			quote,
			api.Mul(flags[slot].deposit, message.TokenID, message.DepositAmount),
			64,
		)
	}
	return base, quote
}

func selectAccountBalance(
	api frontend.API,
	accounts [protocol.AccountLeafCount]AccountWitness,
	accountID frontend.Variable,
	quote bool,
) frontend.Variable {
	g := NewGadgets(api)
	selected := frontend.Variable(0)
	for index, account := range accounts {
		balance := account.BaseBalance
		if quote {
			balance = account.QuoteBalance
		}
		selected = api.Add(selected, api.Mul(g.IsEqual(accountID, index), balance))
	}
	g.AssertUnsigned(selected, 64)
	return selected
}

func constrainClearing(
	api frontend.API,
	orders [protocol.MaxSlots]constrainedOrder,
	preAMM AMMWitness,
) (clearingCandidate, error) {
	winner := clearingCandidate{
		valid:             0,
		tick:              0,
		demand:            0,
		supply:            0,
		internalMatched:   0,
		requestedResidual: 0,
		direction:         0,
		ammFilled:         0,
		executableBase:    0,
		spotDistance:      0,
		postBaseReserve:   preAMM.BaseReserve,
		postQuoteReserve:  preAMM.QuoteReserve,
	}
	for slot := range orders {
		candidate, err := constrainCandidate(
			api,
			orders[slot].limitTick,
			orders[slot].active,
			orders,
			preAMM,
		)
		if err != nil {
			return clearingCandidate{}, err
		}
		winner = selectBetterCandidate(api, candidate, winner)
	}
	return winner, nil
}

func constrainCandidate(
	api frontend.API,
	tick frontend.Variable,
	valid frontend.Variable,
	orders [protocol.MaxSlots]constrainedOrder,
	preAMM AMMWitness,
) (clearingCandidate, error) {
	g := NewGadgets(api)
	g.AssertBoolean(valid)
	g.AssertUnsigned(tick, 8)
	demand := frontend.Variable(0)
	supply := frontend.Variable(0)
	for _, order := range orders {
		buyEligible := api.Sub(1, g.IsLess(order.limitTick, tick, 8))
		sellEligible := api.Sub(1, g.IsLess(tick, order.limitTick, 8))
		demand = g.AddNoOverflow(
			demand,
			api.Mul(order.activeBuy, buyEligible, order.baseAmount),
			35,
		)
		supply = g.AddNoOverflow(
			supply,
			api.Mul(order.activeSell, sellEligible, order.baseAmount),
			35,
		)
	}
	internalMatched := g.Min(demand, supply, 35)
	requested := api.Sub(api.Add(demand, supply), api.Mul(2, internalMatched))
	g.AssertUnsigned(requested, 35)
	demandExcess := g.IsLess(supply, demand, 35)
	supplyExcess := g.IsLess(demand, supply, 35)
	direction := api.Add(demandExcess, api.Mul(2, supplyExcess))
	g.AssertUnsigned(direction, 2)
	g.AssertEnum(direction, 0, 1, 2)

	price := api.Add(tick, 1)
	g.AssertUnsigned(price, 9)
	ammResult, err := constrainMaximumResidual(
		api,
		preAMM,
		price,
		requested,
		direction,
	)
	if err != nil {
		return clearingCandidate{}, err
	}
	doubledMatched := api.Mul(2, internalMatched)
	g.AssertUnsigned(doubledMatched, 36)
	executable := g.AddNoOverflow(doubledMatched, ammResult.filled, 36)
	spotDistance := constrainSpotDistance(api, price, preAMM)
	return clearingCandidate{
		valid:             valid,
		tick:              tick,
		demand:            demand,
		supply:            supply,
		internalMatched:   internalMatched,
		requestedResidual: requested,
		direction:         direction,
		ammFilled:         ammResult.filled,
		executableBase:    executable,
		spotDistance:      spotDistance,
		postBaseReserve:   ammResult.postBase,
		postQuoteReserve:  ammResult.postQuote,
	}, nil
}

func constrainSpotDistance(
	api frontend.API,
	price frontend.Variable,
	preAMM AMMWitness,
) frontend.Variable {
	g := NewGadgets(api)
	product := g.MulBounded(price, 9, preAMM.BaseReserve, 64, 73)
	quoteBelowProduct := g.IsLess(preAMM.QuoteReserve, product, 73)
	difference := g.Select(
		quoteBelowProduct,
		api.Sub(product, preAMM.QuoteReserve),
		api.Sub(preAMM.QuoteReserve, product),
	)
	g.AssertUnsigned(difference, 73)
	return difference
}

func selectBetterCandidate(
	api frontend.API,
	candidate clearingCandidate,
	incumbent clearingCandidate,
) clearingCandidate {
	g := NewGadgets(api)
	executableGreater := g.IsLess(
		incumbent.executableBase,
		candidate.executableBase,
		36,
	)
	executableEqual := g.IsEqual(candidate.executableBase, incumbent.executableBase)
	matchedGreater := g.IsLess(
		incumbent.internalMatched,
		candidate.internalMatched,
		35,
	)
	matchedEqual := g.IsEqual(candidate.internalMatched, incumbent.internalMatched)
	distanceLess := g.IsLess(candidate.spotDistance, incumbent.spotDistance, 73)
	distanceEqual := g.IsEqual(candidate.spotDistance, incumbent.spotDistance)
	tickLess := g.IsLess(candidate.tick, incumbent.tick, 8)

	distanceOrTick := api.Add(distanceLess, api.Mul(distanceEqual, tickLess))
	matchedOrLater := api.Add(
		matchedGreater,
		api.Mul(matchedEqual, distanceOrTick),
	)
	better := api.Add(
		executableGreater,
		api.Mul(executableEqual, matchedOrLater),
	)
	g.AssertBoolean(better)
	choose := api.Mul(
		candidate.valid,
		api.Add(api.Sub(1, incumbent.valid), api.Mul(incumbent.valid, better)),
	)
	g.AssertBoolean(choose)
	return clearingCandidate{
		valid:             g.Select(choose, candidate.valid, incumbent.valid),
		tick:              g.Select(choose, candidate.tick, incumbent.tick),
		demand:            g.Select(choose, candidate.demand, incumbent.demand),
		supply:            g.Select(choose, candidate.supply, incumbent.supply),
		internalMatched:   g.Select(choose, candidate.internalMatched, incumbent.internalMatched),
		requestedResidual: g.Select(choose, candidate.requestedResidual, incumbent.requestedResidual),
		direction:         g.Select(choose, candidate.direction, incumbent.direction),
		ammFilled:         g.Select(choose, candidate.ammFilled, incumbent.ammFilled),
		executableBase:    g.Select(choose, candidate.executableBase, incumbent.executableBase),
		spotDistance:      g.Select(choose, candidate.spotDistance, incumbent.spotDistance),
		postBaseReserve:   g.Select(choose, candidate.postBaseReserve, incumbent.postBaseReserve),
		postQuoteReserve:  g.Select(choose, candidate.postQuoteReserve, incumbent.postQuoteReserve),
	}
}

func constrainFIFO(
	api frontend.API,
	orders [protocol.MaxSlots]constrainedOrder,
	winner clearingCandidate,
) [protocol.MaxSlots]frontend.Variable {
	g := NewGadgets(api)
	directionSellsBase := g.IsEqual(winner.direction, 1)
	directionBuysBase := g.IsEqual(winner.direction, 2)
	buyRemaining := g.AddNoOverflow(
		winner.internalMatched,
		api.Mul(directionSellsBase, winner.ammFilled),
		35,
	)
	sellRemaining := g.AddNoOverflow(
		winner.internalMatched,
		api.Mul(directionBuysBase, winner.ammFilled),
		35,
	)

	var fills [protocol.MaxSlots]frontend.Variable
	for slot, order := range orders {
		buyEligible := api.Sub(1, g.IsLess(order.limitTick, winner.tick, 8))
		sellEligible := api.Sub(1, g.IsLess(winner.tick, order.limitTick, 8))
		buyFill := api.Mul(
			order.activeBuy,
			buyEligible,
			g.Min(order.baseAmount, buyRemaining, 35),
		)
		sellFill := api.Mul(
			order.activeSell,
			sellEligible,
			g.Min(order.baseAmount, sellRemaining, 35),
		)
		fills[slot] = api.Add(buyFill, sellFill)
		g.AssertUnsigned(fills[slot], 32)
		buyRemaining = g.SubNoUnderflow(buyRemaining, buyFill, 35)
		sellRemaining = g.SubNoUnderflow(sellRemaining, sellFill, 35)
	}
	api.AssertIsEqual(buyRemaining, 0)
	api.AssertIsEqual(sellRemaining, 0)
	return fills
}

func constrainSettlement(
	api frontend.API,
	accounts [protocol.AccountLeafCount]AccountWitness,
	orders [protocol.MaxSlots]constrainedOrder,
	fills [protocol.MaxSlots]frontend.Variable,
	clearingTick frontend.Variable,
) [protocol.AccountLeafCount]AccountWitness {
	g := NewGadgets(api)
	price := api.Add(clearingTick, 1)
	g.AssertUnsigned(price, 9)
	for slot, order := range orders {
		spent := g.MulBounded(fills[slot], 32, price, 9, 41)
		buySpent := api.Mul(order.activeBuy, spent)
		sellProceeds := api.Mul(order.activeSell, spent)
		buyRefund := g.SubNoUnderflow(order.buyReserved, buySpent, 64)
		sellRefund := g.SubNoUnderflow(
			order.sellReserved,
			api.Mul(order.activeSell, fills[slot]),
			64,
		)
		baseCredit := api.Add(
			api.Mul(order.activeBuy, fills[slot]),
			sellRefund,
		)
		quoteCredit := api.Add(buyRefund, sellProceeds)
		for account := range accounts {
			target := g.IsEqual(order.accountID, account)
			accounts[account].BaseBalance = g.AddNoOverflow(
				accounts[account].BaseBalance,
				api.Mul(target, baseCredit),
				64,
			)
			accounts[account].QuoteBalance = g.AddNoOverflow(
				accounts[account].QuoteBalance,
				api.Mul(target, quoteCredit),
				64,
			)
		}
	}
	return accounts
}
