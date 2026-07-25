// Package protocol owns the canonical FIFO-Clear protocol constants, message
// model, validation rules, and batch byte encoding.
package protocol

import "math"

const (
	MaxSlots            = 8
	TreeDepth           = 4
	LeafCount           = 16
	AccountLeafCount    = MaxSlots
	AMMLeafIndex        = 8
	MetadataLeafIndex   = 9
	FirstEmptyLeafIndex = 10

	MaxAccountID     = MaxSlots - 1
	MaxBatchIndex    = (uint64(1) << 61) - 1
	MaxDepositAmount = math.MaxUint64
	MaxOrderAmount   = math.MaxUint32
	MaxLimitTick     = math.MaxUint8
)

type MessageType uint8

const (
	MessageTypeEmpty MessageType = iota
	MessageTypeDeposit
	MessageTypeOrder
)

type TokenID uint8

const (
	TokenBase TokenID = iota
	TokenQuote
)

type Side uint8

const (
	SideBuy Side = iota
	SideSell
)

type FundingStatus uint8

const (
	FundingStatusNotOrder FundingStatus = iota
	FundingStatusRejectedUnfunded
	FundingStatusActive
)

// Message is a fixed union. Fields irrelevant to the selected MessageType must
// be canonical zero.
type Message struct {
	Type          MessageType
	SequenceID    uint64
	AccountID     uint64
	TokenID       TokenID
	DepositAmount uint64
	Side          Side
	BaseAmount    uint64
	LimitTick     uint64
}

type Batch struct {
	BatchIndex      uint64
	StartSequenceID uint64
	MessageCount    uint8
	Slots           [MaxSlots]Message
}

func (messageType MessageType) String() string {
	switch messageType {
	case MessageTypeEmpty:
		return "EMPTY"
	case MessageTypeDeposit:
		return "DEPOSIT"
	case MessageTypeOrder:
		return "ORDER"
	default:
		return "UNKNOWN"
	}
}

func (tokenID TokenID) String() string {
	switch tokenID {
	case TokenBase:
		return "BASE"
	case TokenQuote:
		return "QUOTE"
	default:
		return "UNKNOWN"
	}
}

func (side Side) String() string {
	switch side {
	case SideBuy:
		return "BUY"
	case SideSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}
