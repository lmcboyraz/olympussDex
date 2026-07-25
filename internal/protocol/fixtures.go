package protocol

func FullBatchFixture() Batch {
	return Batch{
		BatchIndex:      7,
		StartSequenceID: 100,
		MessageCount:    MaxSlots,
		Slots: [MaxSlots]Message{
			deposit(100, 0, TokenBase, 500),
			deposit(101, 1, TokenQuote, 1_250),
			order(102, 0, SideBuy, 10, 99),
			order(103, 1, SideSell, 7, 101),
			deposit(104, 2, TokenQuote, 999),
			order(105, 3, SideBuy, 3, 100),
			deposit(106, 7, TokenBase, 1),
			order(107, 7, SideSell, 2, 255),
		},
	}
}

func PartialBatchFixture() Batch {
	return Batch{
		BatchIndex:      8,
		StartSequenceID: 108,
		MessageCount:    1,
		Slots: [MaxSlots]Message{
			deposit(108, 4, TokenQuote, 42),
		},
	}
}

func deposit(sequenceID, accountID uint64, tokenID TokenID, amount uint64) Message {
	return Message{
		Type:          MessageTypeDeposit,
		SequenceID:    sequenceID,
		AccountID:     accountID,
		TokenID:       tokenID,
		DepositAmount: amount,
	}
}

func order(
	sequenceID uint64,
	accountID uint64,
	side Side,
	baseAmount uint64,
	limitTick uint64,
) Message {
	return Message{
		Type:       MessageTypeOrder,
		SequenceID: sequenceID,
		AccountID:  accountID,
		Side:       side,
		BaseAmount: baseAmount,
		LimitTick:  limitTick,
	}
}
