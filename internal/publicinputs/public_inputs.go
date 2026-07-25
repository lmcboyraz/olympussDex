// Package publicinputs owns the canonical ordering of the FIFO-Clear public
// inputs shared by the Go prover and the generated Solidity verifier.
package publicinputs

const (
	BatchIndex = iota
	MessageCount
	OldStateRoot
	BatchCommitmentHi
	BatchCommitmentLo
	NewStateRoot
	ClearingTick
	fundingStatusStart
	fundingStatusEnd      = fundingStatusStart + GroupSize
	filledBaseAmountStart = fundingStatusEnd
	filledBaseAmountEnd   = filledBaseAmountStart + GroupSize
	InternalMatchedBase   = filledBaseAmountEnd
	RequestedResidualBase = InternalMatchedBase + 1
	AMMResidualDirection  = RequestedResidualBase + 1
	AMMFilledBase         = AMMResidualDirection + 1
	Count                 = AMMFilledBase + 1
)

const GroupSize = 8

var orderedNames = [Count]string{
	"batchIndex",
	"messageCount",
	"oldStateRoot",
	"batchCommitmentHi",
	"batchCommitmentLo",
	"newStateRoot",
	"clearingTick",
	"fundingStatus[0]",
	"fundingStatus[1]",
	"fundingStatus[2]",
	"fundingStatus[3]",
	"fundingStatus[4]",
	"fundingStatus[5]",
	"fundingStatus[6]",
	"fundingStatus[7]",
	"filledBaseAmount[0]",
	"filledBaseAmount[1]",
	"filledBaseAmount[2]",
	"filledBaseAmount[3]",
	"filledBaseAmount[4]",
	"filledBaseAmount[5]",
	"filledBaseAmount[6]",
	"filledBaseAmount[7]",
	"internalMatchedBase",
	"requestedResidualBase",
	"ammResidualDirection",
	"ammFilledBase",
}

// Names returns a copy so callers cannot mutate the canonical definition.
func Names() []string {
	names := make([]string, Count)
	copy(names, orderedNames[:])
	return names
}

func FundingStatus(position int) int {
	if position < 0 || position >= GroupSize {
		panic("funding status position out of range")
	}
	return fundingStatusStart + position
}

func FilledBaseAmount(position int) int {
	if position < 0 || position >= GroupSize {
		panic("filled base amount position out of range")
	}
	return filledBaseAmountStart + position
}
