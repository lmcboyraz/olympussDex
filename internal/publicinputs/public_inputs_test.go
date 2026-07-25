package publicinputs

import (
	"slices"
	"testing"
)

func TestCanonicalOrderMatchesFinalABI(t *testing.T) {
	want := []string{
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

	if Count != len(want) {
		t.Fatalf("Count = %d, want %d", Count, len(want))
	}
	if got := Names(); !slices.Equal(got, want) {
		t.Fatalf("canonical order = %v, want %v", got, want)
	}
}

func TestGroupedIndexesMatchFinalABI(t *testing.T) {
	for i := range GroupSize {
		if got, want := FundingStatus(i), 7+i; got != want {
			t.Fatalf("FundingStatus(%d) = %d, want %d", i, got, want)
		}
		if got, want := FilledBaseAmount(i), 15+i; got != want {
			t.Fatalf("FilledBaseAmount(%d) = %d, want %d", i, got, want)
		}
	}
}
