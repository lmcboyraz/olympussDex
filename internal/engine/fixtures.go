package engine

import "github.com/cemilboyraz/oly2/internal/protocol"

// ShowcaseBatch0 is the canonical excess-BUY reference transition.
func ShowcaseBatch0() protocol.Batch {
	return protocol.Batch{
		BatchIndex:      0,
		StartSequenceID: 0,
		MessageCount:    protocol.MaxSlots,
		Slots: [protocol.MaxSlots]protocol.Message{
			orderMessage(0, 0, protocol.SideBuy, 50, 109),
			orderMessage(1, 1, protocol.SideBuy, 50, 109),
			orderMessage(2, 2, protocol.SideBuy, 50, 109),
			orderMessage(3, 3, protocol.SideBuy, 50, 109),
			orderMessage(4, 4, protocol.SideSell, 20, 109),
			orderMessage(5, 5, protocol.SideSell, 20, 109),
			orderMessage(6, 6, protocol.SideSell, 20, 109),
			orderMessage(7, 7, protocol.SideSell, 20, 109),
		},
	}
}

// ShowcaseBatch1 is the canonical excess-SELL reference transition. It must
// execute against ShowcaseBatch0's post-state.
func ShowcaseBatch1() protocol.Batch {
	return protocol.Batch{
		BatchIndex:      1,
		StartSequenceID: 8,
		MessageCount:    protocol.MaxSlots,
		Slots: [protocol.MaxSlots]protocol.Message{
			orderMessage(8, 0, protocol.SideBuy, 20, 109),
			orderMessage(9, 1, protocol.SideBuy, 20, 109),
			orderMessage(10, 2, protocol.SideBuy, 20, 109),
			orderMessage(11, 4, protocol.SideSell, 60, 109),
			orderMessage(12, 5, protocol.SideSell, 40, 109),
			orderMessage(13, 6, protocol.SideSell, 40, 109),
			orderMessage(14, 7, protocol.SideSell, 40, 109),
			orderMessage(15, 4, protocol.SideSell, 50, 109),
		},
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
