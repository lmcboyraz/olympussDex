// Package circuit implements the production FIFO-Clear gnark circuit.
package circuit

import (
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark/frontend"
)

// AccountWitness is one private account leaf.
type AccountWitness struct {
	BaseBalance  frontend.Variable
	QuoteBalance frontend.Variable
}

// AMMWitness is the private pre-state AMM leaf.
type AMMWitness struct {
	BaseReserve  frontend.Variable
	QuoteReserve frontend.Variable
}

// MetadataWitness is the private state-chain metadata leaf.
type MetadataWitness struct {
	ProcessedBatchCount   frontend.Variable
	ProcessedMessageCount frontend.Variable
}

// StateWitness is the complete private 16-leaf state preimage. Empty leaves are
// protocol constants and therefore do not appear in the witness.
type StateWitness struct {
	Accounts [protocol.AccountLeafCount]AccountWitness
	AMM      AMMWitness
	Metadata MetadataWitness
}

// MessageWitness is one private canonical message-union slot.
type MessageWitness struct {
	Type          frontend.Variable
	SequenceID    frontend.Variable
	AccountID     frontend.Variable
	TokenID       frontend.Variable
	DepositAmount frontend.Variable
	Side          frontend.Variable
	BaseAmount    frontend.Variable
	LimitTick     frontend.Variable
}

// BatchWitness is the complete private canonical eight-slot batch.
type BatchWitness struct {
	BatchIndex      frontend.Variable
	StartSequenceID frontend.Variable
	MessageCount    frontend.Variable
	Slots           [protocol.MaxSlots]MessageWitness
}

// PublicWitness owns the canonical 27-input gnark array. Its indexes are only
// interpreted through internal/publicinputs.
type PublicWitness struct {
	Inputs [publicinputs.Count]frontend.Variable `gnark:",public"`
}

// AssignState converts the native state into a circuit witness.
func AssignState(value state.State) StateWitness {
	var witness StateWitness
	for index, account := range value.Accounts {
		witness.Accounts[index] = AccountWitness{
			BaseBalance:  account.BaseBalance,
			QuoteBalance: account.QuoteBalance,
		}
	}
	witness.AMM = AMMWitness{
		BaseReserve:  value.AMM.BaseReserve,
		QuoteReserve: value.AMM.QuoteReserve,
	}
	witness.Metadata = MetadataWitness{
		ProcessedBatchCount:   value.Metadata.ProcessedBatchCount,
		ProcessedMessageCount: value.Metadata.ProcessedMessageCount,
	}
	return witness
}

// AssignBatch converts the native canonical batch into a circuit witness.
func AssignBatch(value protocol.Batch) BatchWitness {
	witness := BatchWitness{
		BatchIndex:      value.BatchIndex,
		StartSequenceID: value.StartSequenceID,
		MessageCount:    value.MessageCount,
	}
	for index, message := range value.Slots {
		witness.Slots[index] = MessageWitness{
			Type:          uint8(message.Type),
			SequenceID:    message.SequenceID,
			AccountID:     message.AccountID,
			TokenID:       uint8(message.TokenID),
			DepositAmount: message.DepositAmount,
			Side:          uint8(message.Side),
			BaseAmount:    message.BaseAmount,
			LimitTick:     message.LimitTick,
		}
	}
	return witness
}
