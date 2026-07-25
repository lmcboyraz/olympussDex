package protocol

import (
	"fmt"
	"math"
)

func (batch Batch) Validate() error {
	if batch.BatchIndex > MaxBatchIndex {
		return fmt.Errorf("batch index %d exceeds 61 bits", batch.BatchIndex)
	}
	if batch.MessageCount < 1 || batch.MessageCount > MaxSlots {
		return fmt.Errorf("message count %d is outside [1,%d]", batch.MessageCount, MaxSlots)
	}

	messageCount := int(batch.MessageCount)
	for position, message := range batch.Slots {
		if position < messageCount {
			if message.Type == MessageTypeEmpty {
				return fmt.Errorf("slot %d is EMPTY inside the real-message prefix", position)
			}
			if batch.StartSequenceID > math.MaxUint64-uint64(position) {
				return fmt.Errorf("sequence ID overflows at slot %d", position)
			}
			expectedSequenceID := batch.StartSequenceID + uint64(position)
			if message.SequenceID != expectedSequenceID {
				return fmt.Errorf(
					"slot %d sequence ID = %d, want %d",
					position,
					message.SequenceID,
					expectedSequenceID,
				)
			}
		} else if message.Type != MessageTypeEmpty {
			return fmt.Errorf("slot %d is a real message after messageCount", position)
		}

		if err := message.validate(position); err != nil {
			return err
		}
	}
	return nil
}

func (message Message) validate(position int) error {
	if (message.Type == MessageTypeDeposit || message.Type == MessageTypeOrder) &&
		message.AccountID > MaxAccountID {
		return fmt.Errorf("slot %d account ID %d is invalid", position, message.AccountID)
	}

	switch message.Type {
	case MessageTypeEmpty:
		if message.SequenceID != 0 ||
			message.AccountID != 0 ||
			message.TokenID != 0 ||
			message.DepositAmount != 0 ||
			message.Side != 0 ||
			message.BaseAmount != 0 ||
			message.LimitTick != 0 {
			return fmt.Errorf("slot %d EMPTY message is not canonical zero", position)
		}
	case MessageTypeDeposit:
		if message.TokenID != TokenBase && message.TokenID != TokenQuote {
			return fmt.Errorf("slot %d token ID %d is invalid", position, message.TokenID)
		}
		if message.DepositAmount == 0 {
			return fmt.Errorf("slot %d deposit amount must be positive", position)
		}
		if message.Side != 0 || message.BaseAmount != 0 || message.LimitTick != 0 {
			return fmt.Errorf("slot %d DEPOSIT order fields are not canonical zero", position)
		}
	case MessageTypeOrder:
		if message.Side != SideBuy && message.Side != SideSell {
			return fmt.Errorf("slot %d side %d is invalid", position, message.Side)
		}
		if message.BaseAmount == 0 || message.BaseAmount > MaxOrderAmount {
			return fmt.Errorf("slot %d base amount %d is outside uint32 positive range", position, message.BaseAmount)
		}
		if message.LimitTick > MaxLimitTick {
			return fmt.Errorf("slot %d limit tick %d exceeds uint8", position, message.LimitTick)
		}
		if message.TokenID != 0 || message.DepositAmount != 0 {
			return fmt.Errorf("slot %d ORDER deposit fields are not canonical zero", position)
		}
	default:
		return fmt.Errorf("slot %d message type %d is invalid", position, message.Type)
	}
	return nil
}
