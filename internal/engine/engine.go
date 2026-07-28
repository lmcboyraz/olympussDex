// Package engine implements the deterministic FIFO-Clear Go reference
// transition. Reservations exist only while Execute is running; the returned
// state contains settled available balances only.
package engine

import (
	"errors"
	"fmt"
	"math/big"
	"math/bits"

	"github.com/cemilboyraz/oly2/internal/engine/amm"
	"github.com/cemilboyraz/oly2/internal/engine/clearing"
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// ErrGlobalLiabilityCapExceeded identifies a pre-state, deposit set, expected
// total, or post-state whose per-token aggregate exceeds uint64 maximum.
var ErrGlobalLiabilityCapExceeded = errors.New("global liability cap exceeded")

type DepositTotals struct {
	Base  uint64
	Quote uint64
}

type wideUint struct {
	high uint64
	low  uint64
}

type wideLiabilities struct {
	base  wideUint
	quote wideUint
}

type Result struct {
	BatchIndex      uint64
	MessageCount    uint8
	PostState       state.State
	OldStateRoot    fr.Element
	NewStateRoot    fr.Element
	BatchCommitment protocol.Commitment

	FundingStatus    [protocol.MaxSlots]protocol.FundingStatus
	FilledBaseAmount [protocol.MaxSlots]uint64

	ClearingTick          uint8
	Demand                uint64
	Supply                uint64
	InternalMatchedBase   uint64
	RequestedResidualBase uint64
	AMMResidualDirection  amm.Direction
	AMMFilledBase         uint64
	UnfilledResidualBase  uint64
}

type reservedOrder struct {
	clearing.Order
	accountIndex int
	reserved     uint64
}

func Execute(pre state.State, batch protocol.Batch) (Result, error) {
	if err := batch.Validate(); err != nil {
		return Result{}, fmt.Errorf("validate batch: %w", err)
	}
	if pre.Metadata.ProcessedBatchCount != batch.BatchIndex {
		return Result{}, fmt.Errorf(
			"batch index %d does not match processed batch count %d",
			batch.BatchIndex,
			pre.Metadata.ProcessedBatchCount,
		)
	}
	if pre.Metadata.ProcessedMessageCount != batch.StartSequenceID {
		return Result{}, fmt.Errorf(
			"start sequence %d does not match processed message count %d",
			batch.StartSequenceID,
			pre.Metadata.ProcessedMessageCount,
		)
	}

	deposits, err := validatePreStateAndDeposits(pre, batch)
	if err != nil {
		return Result{}, err
	}
	oldRoot, err := stateRoot(pre)
	if err != nil {
		return Result{}, fmt.Errorf("old state root: %w", err)
	}
	commitment, err := protocol.CommitBatch(batch)
	if err != nil {
		return Result{}, fmt.Errorf("commit batch: %w", err)
	}

	result := Result{
		BatchIndex:      batch.BatchIndex,
		MessageCount:    batch.MessageCount,
		PostState:       pre,
		OldStateRoot:    oldRoot,
		BatchCommitment: commitment,
	}
	activeOrders := make([]reservedOrder, 0, batch.MessageCount)
	for slot := 0; slot < int(batch.MessageCount); slot++ {
		message := batch.Slots[slot]
		switch message.Type {
		case protocol.MessageTypeDeposit:
			if err := applyDeposit(&result.PostState, message); err != nil {
				return Result{}, fmt.Errorf("slot %d deposit: %w", slot, err)
			}
		case protocol.MessageTypeOrder:
			order, active, err := reserveOrder(&result.PostState, slot, message)
			if err != nil {
				return Result{}, fmt.Errorf("slot %d order reservation: %w", slot, err)
			}
			if active {
				result.FundingStatus[slot] = protocol.FundingStatusActive
				activeOrders = append(activeOrders, order)
			} else {
				result.FundingStatus[slot] = protocol.FundingStatusRejectedUnfunded
			}
		default:
			return Result{}, fmt.Errorf("slot %d has unexpected message type %d", slot, message.Type)
		}
	}

	orders := make([]clearing.Order, len(activeOrders))
	for index := range activeOrders {
		orders[index] = activeOrders[index].Order
	}
	clearingResult, err := clearing.Evaluate(
		orders,
		amm.Reserves{Base: pre.AMM.BaseReserve, Quote: pre.AMM.QuoteReserve},
	)
	if err != nil {
		return Result{}, fmt.Errorf("evaluate clearing: %w", err)
	}
	result.ClearingTick = clearingResult.ClearingTick
	result.Demand = clearingResult.Demand
	result.Supply = clearingResult.Supply
	result.InternalMatchedBase = clearingResult.InternalMatched
	result.RequestedResidualBase = clearingResult.RequestedResidual
	result.AMMResidualDirection = clearingResult.Direction
	result.AMMFilledBase = clearingResult.AMMFilled
	result.UnfilledResidualBase = clearingResult.UnfilledResidual

	if err := allocateFIFO(activeOrders, clearingResult, &result.FilledBaseAmount); err != nil {
		return Result{}, fmt.Errorf("allocate FIFO fills: %w", err)
	}
	if err := settleOrders(&result.PostState, activeOrders, result.FilledBaseAmount, result.ClearingTick); err != nil {
		return Result{}, fmt.Errorf("settle orders: %w", err)
	}
	if result.AMMFilledBase > 0 {
		postAMM, err := amm.Apply(
			amm.Reserves{Base: pre.AMM.BaseReserve, Quote: pre.AMM.QuoteReserve},
			uint64(result.ClearingTick)+1,
			result.AMMFilledBase,
			result.AMMResidualDirection,
		)
		if err != nil {
			return Result{}, fmt.Errorf("apply AMM residual: %w", err)
		}
		result.PostState.AMM = state.AMM{
			BaseReserve:  postAMM.Base,
			QuoteReserve: postAMM.Quote,
		}
	}

	result.PostState.Metadata.ProcessedBatchCount, err = checkedAdd(
		pre.Metadata.ProcessedBatchCount,
		1,
	)
	if err != nil {
		return Result{}, fmt.Errorf("processed batch count: %w", err)
	}
	result.PostState.Metadata.ProcessedMessageCount, err = checkedAdd(
		pre.Metadata.ProcessedMessageCount,
		uint64(batch.MessageCount),
	)
	if err != nil {
		return Result{}, fmt.Errorf("processed message count: %w", err)
	}

	if err := CheckConservation(pre, result.PostState, deposits); err != nil {
		return Result{}, err
	}
	result.NewStateRoot, err = stateRoot(result.PostState)
	if err != nil {
		return Result{}, fmt.Errorf("new state root: %w", err)
	}
	return result, nil
}

func (result Result) PublicInputs() [publicinputs.Count]*big.Int {
	var inputs [publicinputs.Count]*big.Int
	for index := range inputs {
		inputs[index] = new(big.Int)
	}

	inputs[publicinputs.BatchIndex].SetUint64(result.BatchIndex)
	inputs[publicinputs.MessageCount].SetUint64(uint64(result.MessageCount))
	result.OldStateRoot.BigInt(inputs[publicinputs.OldStateRoot])
	inputs[publicinputs.BatchCommitmentHi].SetBytes(result.BatchCommitment.Hi[:])
	inputs[publicinputs.BatchCommitmentLo].SetBytes(result.BatchCommitment.Lo[:])
	result.NewStateRoot.BigInt(inputs[publicinputs.NewStateRoot])
	inputs[publicinputs.ClearingTick].SetUint64(uint64(result.ClearingTick))
	for index := 0; index < protocol.MaxSlots; index++ {
		inputs[publicinputs.FundingStatus(index)].SetUint64(uint64(result.FundingStatus[index]))
		inputs[publicinputs.FilledBaseAmount(index)].SetUint64(result.FilledBaseAmount[index])
	}
	inputs[publicinputs.InternalMatchedBase].SetUint64(result.InternalMatchedBase)
	inputs[publicinputs.RequestedResidualBase].SetUint64(result.RequestedResidualBase)
	inputs[publicinputs.AMMResidualDirection].SetUint64(uint64(result.AMMResidualDirection))
	inputs[publicinputs.AMMFilledBase].SetUint64(result.AMMFilledBase)
	return inputs
}

func applyDeposit(
	post *state.State,
	message protocol.Message,
) error {
	account := &post.Accounts[int(message.AccountID)]
	var err error
	switch message.TokenID {
	case protocol.TokenBase:
		account.BaseBalance, err = checkedAdd(account.BaseBalance, message.DepositAmount)
	case protocol.TokenQuote:
		account.QuoteBalance, err = checkedAdd(account.QuoteBalance, message.DepositAmount)
	default:
		return fmt.Errorf("invalid token %d", message.TokenID)
	}
	return err
}

func reserveOrder(
	post *state.State,
	slot int,
	message protocol.Message,
) (reservedOrder, bool, error) {
	order := reservedOrder{
		Order: clearing.Order{
			Slot:       slot,
			Side:       message.Side,
			BaseAmount: message.BaseAmount,
			LimitTick:  uint8(message.LimitTick),
		},
		accountIndex: int(message.AccountID),
	}
	account := &post.Accounts[order.accountIndex]
	switch message.Side {
	case protocol.SideBuy:
		required, err := checkedMul(message.BaseAmount, message.LimitTick+1)
		if err != nil {
			return reservedOrder{}, false, err
		}
		if account.QuoteBalance < required {
			return order, false, nil
		}
		account.QuoteBalance -= required
		order.reserved = required
	case protocol.SideSell:
		if account.BaseBalance < message.BaseAmount {
			return order, false, nil
		}
		account.BaseBalance -= message.BaseAmount
		order.reserved = message.BaseAmount
	default:
		return reservedOrder{}, false, fmt.Errorf("invalid side %d", message.Side)
	}
	return order, true, nil
}

func allocateFIFO(
	orders []reservedOrder,
	trace clearing.Result,
	fills *[protocol.MaxSlots]uint64,
) error {
	if len(orders) == 0 {
		return nil
	}
	buyCapacity := trace.InternalMatched
	sellCapacity := trace.InternalMatched
	var err error
	switch trace.Direction {
	case amm.DirectionSellsBase:
		buyCapacity, err = checkedAdd(buyCapacity, trace.AMMFilled)
	case amm.DirectionBuysBase:
		sellCapacity, err = checkedAdd(sellCapacity, trace.AMMFilled)
	case amm.DirectionNone:
	default:
		return fmt.Errorf("invalid AMM direction %d", trace.Direction)
	}
	if err != nil {
		return err
	}

	for _, order := range orders {
		if !eligible(order.Order, trace.ClearingTick) {
			continue
		}
		switch order.Side {
		case protocol.SideBuy:
			fill := min(order.BaseAmount, buyCapacity)
			fills[order.Slot] = fill
			buyCapacity -= fill
		case protocol.SideSell:
			fill := min(order.BaseAmount, sellCapacity)
			fills[order.Slot] = fill
			sellCapacity -= fill
		default:
			return fmt.Errorf("slot %d has invalid side %d", order.Slot, order.Side)
		}
	}
	if buyCapacity != 0 || sellCapacity != 0 {
		return fmt.Errorf(
			"trace capacities not allocated: BUY=%d SELL=%d",
			buyCapacity,
			sellCapacity,
		)
	}
	return nil
}

func settleOrders(
	post *state.State,
	orders []reservedOrder,
	fills [protocol.MaxSlots]uint64,
	clearingTick uint8,
) error {
	price := uint64(clearingTick) + 1
	for _, order := range orders {
		fill := fills[order.Slot]
		if fill > order.BaseAmount {
			return fmt.Errorf("slot %d fill exceeds order amount", order.Slot)
		}
		account := &post.Accounts[order.accountIndex]
		switch order.Side {
		case protocol.SideBuy:
			spent, err := checkedMul(fill, price)
			if err != nil {
				return fmt.Errorf("slot %d BUY quote: %w", order.Slot, err)
			}
			if spent > order.reserved {
				return fmt.Errorf("slot %d BUY spent quote exceeds reservation", order.Slot)
			}
			refund := order.reserved - spent
			account.BaseBalance, err = checkedAdd(account.BaseBalance, fill)
			if err != nil {
				return fmt.Errorf("slot %d BUY base credit: %w", order.Slot, err)
			}
			account.QuoteBalance, err = checkedAdd(account.QuoteBalance, refund)
			if err != nil {
				return fmt.Errorf("slot %d BUY quote refund: %w", order.Slot, err)
			}
		case protocol.SideSell:
			refund := order.reserved - fill
			proceeds, err := checkedMul(fill, price)
			if err != nil {
				return fmt.Errorf("slot %d SELL quote: %w", order.Slot, err)
			}
			account.BaseBalance, err = checkedAdd(account.BaseBalance, refund)
			if err != nil {
				return fmt.Errorf("slot %d SELL base refund: %w", order.Slot, err)
			}
			account.QuoteBalance, err = checkedAdd(account.QuoteBalance, proceeds)
			if err != nil {
				return fmt.Errorf("slot %d SELL quote credit: %w", order.Slot, err)
			}
		default:
			return fmt.Errorf("slot %d has invalid side %d", order.Slot, order.Side)
		}
	}
	return nil
}

func eligible(order clearing.Order, tick uint8) bool {
	switch order.Side {
	case protocol.SideBuy:
		return order.LimitTick >= tick
	case protocol.SideSell:
		return order.LimitTick <= tick
	default:
		return false
	}
}

func CheckConservation(pre state.State, post state.State, deposits DepositTotals) error {
	preTotals, err := stateLiabilitiesWide(pre)
	if err != nil {
		return fmt.Errorf("pre-state liabilities: %w", err)
	}
	if err := validateLiabilityCap("pre-state", preTotals); err != nil {
		return err
	}
	depositTotals := wideLiabilities{
		base:  wideUint{low: deposits.Base},
		quote: wideUint{low: deposits.Quote},
	}
	if err := validateLiabilityCap("batch deposit", depositTotals); err != nil {
		return err
	}
	expectedTotals, err := addLiabilities(preTotals, depositTotals)
	if err != nil {
		return fmt.Errorf("expected liabilities: %w", err)
	}
	if err := validateLiabilityCap("pre-state plus batch deposit", expectedTotals); err != nil {
		return err
	}

	postTotals, err := stateLiabilitiesWide(post)
	if err != nil {
		return fmt.Errorf("post-state liabilities: %w", err)
	}
	if err := validateLiabilityCap("post-state", postTotals); err != nil {
		return err
	}
	if postTotals.base != expectedTotals.base {
		return fmt.Errorf(
			"BASE conservation failed: post=%s expected=%s",
			postTotals.base,
			expectedTotals.base,
		)
	}
	if postTotals.quote != expectedTotals.quote {
		return fmt.Errorf(
			"QUOTE conservation failed: post=%s expected=%s",
			postTotals.quote,
			expectedTotals.quote,
		)
	}
	return nil
}

func validatePreStateAndDeposits(
	pre state.State,
	batch protocol.Batch,
) (DepositTotals, error) {
	preTotals, err := stateLiabilitiesWide(pre)
	if err != nil {
		return DepositTotals{}, fmt.Errorf("pre-state liabilities: %w", err)
	}
	if err := validateLiabilityCap("pre-state", preTotals); err != nil {
		return DepositTotals{}, err
	}

	depositTotals, err := batchDepositTotalsWide(batch)
	if err != nil {
		return DepositTotals{}, fmt.Errorf("batch deposit totals: %w", err)
	}
	if err := validateLiabilityCap("batch deposit", depositTotals); err != nil {
		return DepositTotals{}, err
	}

	expectedTotals, err := addLiabilities(preTotals, depositTotals)
	if err != nil {
		return DepositTotals{}, fmt.Errorf("pre-state plus batch deposit: %w", err)
	}
	if err := validateLiabilityCap("pre-state plus batch deposit", expectedTotals); err != nil {
		return DepositTotals{}, err
	}

	return DepositTotals{
		Base:  depositTotals.base.low,
		Quote: depositTotals.quote.low,
	}, nil
}

func stateLiabilitiesWide(value state.State) (wideLiabilities, error) {
	var totals wideLiabilities
	var err error
	for index, account := range value.Accounts {
		totals.base, err = totals.base.addUint64(account.BaseBalance)
		if err != nil {
			return wideLiabilities{}, fmt.Errorf("account %d BASE: %w", index, err)
		}
		totals.quote, err = totals.quote.addUint64(account.QuoteBalance)
		if err != nil {
			return wideLiabilities{}, fmt.Errorf("account %d QUOTE: %w", index, err)
		}
	}
	totals.base, err = totals.base.addUint64(value.AMM.BaseReserve)
	if err != nil {
		return wideLiabilities{}, fmt.Errorf("AMM BASE: %w", err)
	}
	totals.quote, err = totals.quote.addUint64(value.AMM.QuoteReserve)
	if err != nil {
		return wideLiabilities{}, fmt.Errorf("AMM QUOTE: %w", err)
	}
	return totals, nil
}

func batchDepositTotalsWide(batch protocol.Batch) (wideLiabilities, error) {
	var totals wideLiabilities
	for slot := 0; slot < int(batch.MessageCount); slot++ {
		message := batch.Slots[slot]
		if message.Type != protocol.MessageTypeDeposit {
			continue
		}
		var err error
		switch message.TokenID {
		case protocol.TokenBase:
			totals.base, err = totals.base.addUint64(message.DepositAmount)
		case protocol.TokenQuote:
			totals.quote, err = totals.quote.addUint64(message.DepositAmount)
		default:
			return wideLiabilities{}, fmt.Errorf("slot %d invalid token %d", slot, message.TokenID)
		}
		if err != nil {
			return wideLiabilities{}, fmt.Errorf("slot %d: %w", slot, err)
		}
	}
	return totals, nil
}

func addLiabilities(
	left wideLiabilities,
	right wideLiabilities,
) (wideLiabilities, error) {
	base, err := left.base.add(right.base)
	if err != nil {
		return wideLiabilities{}, fmt.Errorf("BASE: %w", err)
	}
	quote, err := left.quote.add(right.quote)
	if err != nil {
		return wideLiabilities{}, fmt.Errorf("QUOTE: %w", err)
	}
	return wideLiabilities{base: base, quote: quote}, nil
}

func validateLiabilityCap(stage string, totals wideLiabilities) error {
	if !totals.base.fitsUint64() {
		return fmt.Errorf(
			"%w: %s BASE liability exceeds uint64 maximum",
			ErrGlobalLiabilityCapExceeded,
			stage,
		)
	}
	if !totals.quote.fitsUint64() {
		return fmt.Errorf(
			"%w: %s QUOTE liability exceeds uint64 maximum",
			ErrGlobalLiabilityCapExceeded,
			stage,
		)
	}
	return nil
}

func (value wideUint) addUint64(other uint64) (wideUint, error) {
	low, carry := bits.Add64(value.low, other, 0)
	high, overflow := bits.Add64(value.high, 0, carry)
	if overflow != 0 {
		return wideUint{}, fmt.Errorf("uint128 overflow")
	}
	return wideUint{high: high, low: low}, nil
}

func (value wideUint) add(other wideUint) (wideUint, error) {
	low, carry := bits.Add64(value.low, other.low, 0)
	high, overflow := bits.Add64(value.high, other.high, carry)
	if overflow != 0 {
		return wideUint{}, fmt.Errorf("uint128 overflow")
	}
	return wideUint{high: high, low: low}, nil
}

func (value wideUint) fitsUint64() bool {
	return value.high == 0
}

func (value wideUint) String() string {
	if value.high == 0 {
		return new(big.Int).SetUint64(value.low).String()
	}
	result := new(big.Int).SetUint64(value.high)
	result.Lsh(result, 64)
	result.Add(result, new(big.Int).SetUint64(value.low))
	return result.String()
}

func stateRoot(value state.State) (fr.Element, error) {
	leaves, err := value.Leaves()
	if err != nil {
		return fr.Element{}, err
	}
	tree, err := state.BuildTree(leaves[:])
	if err != nil {
		return fr.Element{}, err
	}
	return tree.Root(), nil
}

func checkedAdd(left uint64, right uint64) (uint64, error) {
	sum, carry := bits.Add64(left, right, 0)
	if carry != 0 {
		return 0, fmt.Errorf("uint64 overflow")
	}
	return sum, nil
}

func checkedMul(left uint64, right uint64) (uint64, error) {
	high, low := bits.Mul64(left, right)
	if high != 0 {
		return 0, fmt.Errorf("uint64 overflow")
	}
	return low, nil
}
