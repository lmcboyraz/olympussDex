package protocol

import (
	"bytes"
	"encoding/hex"
	"math"
	"testing"
)

func TestCanonicalFixturesValidateAndEncodeToFixedWidth(t *testing.T) {
	for name, batch := range map[string]Batch{
		"full":    FullBatchFixture(),
		"partial": PartialBatchFixture(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := batch.Validate(); err != nil {
				t.Fatalf("validate fixture: %v", err)
			}
			encoded, err := EncodeBatch(batch)
			if err != nil {
				t.Fatalf("encode fixture: %v", err)
			}
			if len(encoded) != EncodedBatchSize {
				t.Fatalf("encoded length = %d, want %d", len(encoded), EncodedBatchSize)
			}
			domainSeparator := BatchDomainSeparator()
			if !bytes.Equal(encoded[:32], domainSeparator[:]) {
				t.Fatal("encoding does not begin with the batch domain separator")
			}
		})
	}
}

func TestProtocolConstantsAndEnumsMatchCanonicalValues(t *testing.T) {
	if MaxSlots != 8 || TreeDepth != 4 || LeafCount != 16 {
		t.Fatalf("tree constants = (%d,%d,%d)", MaxSlots, TreeDepth, LeafCount)
	}
	if MessageTypeEmpty != 0 || MessageTypeDeposit != 1 || MessageTypeOrder != 2 {
		t.Fatal("message type values changed")
	}
	if TokenBase != 0 || TokenQuote != 1 {
		t.Fatal("token values changed")
	}
	if SideBuy != 0 || SideSell != 1 {
		t.Fatal("side values changed")
	}
	if FundingStatusNotOrder != 0 ||
		FundingStatusRejectedUnfunded != 1 ||
		FundingStatusActive != 2 {
		t.Fatal("funding status values changed")
	}
}

func TestCanonicalValidationRejectsMalformedBatches(t *testing.T) {
	valid := PartialBatchFixture()
	tests := map[string]func(*Batch){
		"zero message count": func(batch *Batch) {
			batch.MessageCount = 0
		},
		"message count above max": func(batch *Batch) {
			batch.MessageCount = MaxSlots + 1
		},
		"batch index above 61 bits": func(batch *Batch) {
			batch.BatchIndex = MaxBatchIndex + 1
		},
		"empty inside real prefix": func(batch *Batch) {
			batch.MessageCount = 2
		},
		"real message after padding begins": func(batch *Batch) {
			batch.Slots[7] = deposit(109, 0, TokenBase, 1)
		},
		"wrong sequence ID": func(batch *Batch) {
			batch.Slots[0].SequenceID++
		},
		"sequence overflow": func(batch *Batch) {
			*batch = FullBatchFixture()
			batch.StartSequenceID = math.MaxUint64 - 6
			for i := range MaxSlots {
				batch.Slots[i].SequenceID = batch.StartSequenceID + uint64(i)
			}
		},
		"invalid message type": func(batch *Batch) {
			batch.Slots[0].Type = MessageType(3)
		},
		"invalid account ID": func(batch *Batch) {
			batch.Slots[0].AccountID = MaxAccountID + 1
		},
		"invalid token ID": func(batch *Batch) {
			batch.Slots[0].TokenID = TokenID(2)
		},
		"zero deposit amount": func(batch *Batch) {
			batch.Slots[0].DepositAmount = 0
		},
		"deposit order field is nonzero": func(batch *Batch) {
			batch.Slots[0].BaseAmount = 1
		},
		"order amount above uint32": func(batch *Batch) {
			*batch = FullBatchFixture()
			batch.Slots[2].BaseAmount = MaxOrderAmount + 1
		},
		"zero order amount": func(batch *Batch) {
			*batch = FullBatchFixture()
			batch.Slots[2].BaseAmount = 0
		},
		"invalid side": func(batch *Batch) {
			*batch = FullBatchFixture()
			batch.Slots[2].Side = Side(2)
		},
		"tick above uint8": func(batch *Batch) {
			*batch = FullBatchFixture()
			batch.Slots[2].LimitTick = MaxLimitTick + 1
		},
		"order deposit field is nonzero": func(batch *Batch) {
			*batch = FullBatchFixture()
			batch.Slots[2].DepositAmount = 1
		},
		"empty payload is nonzero": func(batch *Batch) {
			batch.Slots[1].LimitTick = 1
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			batch := valid
			mutate(&batch)
			if err := batch.Validate(); err == nil {
				t.Fatal("malformed batch unexpectedly validated")
			}
			if _, err := EncodeBatch(batch); err == nil {
				t.Fatal("malformed batch unexpectedly encoded")
			}
		})
	}
}

func TestCanonicalZeroRejectsEveryIrrelevantUnionField(t *testing.T) {
	tests := map[string]struct {
		batch  Batch
		mutate func(*Message)
		slot   int
	}{
		"EMPTY sequence ID": {
			batch:  PartialBatchFixture(),
			slot:   1,
			mutate: func(message *Message) { message.SequenceID = 1 },
		},
		"EMPTY account ID": {
			batch:  PartialBatchFixture(),
			slot:   1,
			mutate: func(message *Message) { message.AccountID = 1 },
		},
		"EMPTY token ID": {
			batch:  PartialBatchFixture(),
			slot:   1,
			mutate: func(message *Message) { message.TokenID = TokenQuote },
		},
		"EMPTY deposit amount": {
			batch:  PartialBatchFixture(),
			slot:   1,
			mutate: func(message *Message) { message.DepositAmount = 1 },
		},
		"EMPTY side": {
			batch:  PartialBatchFixture(),
			slot:   1,
			mutate: func(message *Message) { message.Side = SideSell },
		},
		"EMPTY base amount": {
			batch:  PartialBatchFixture(),
			slot:   1,
			mutate: func(message *Message) { message.BaseAmount = 1 },
		},
		"EMPTY limit tick": {
			batch:  PartialBatchFixture(),
			slot:   1,
			mutate: func(message *Message) { message.LimitTick = 1 },
		},
		"DEPOSIT side": {
			batch:  PartialBatchFixture(),
			slot:   0,
			mutate: func(message *Message) { message.Side = SideSell },
		},
		"DEPOSIT base amount": {
			batch:  PartialBatchFixture(),
			slot:   0,
			mutate: func(message *Message) { message.BaseAmount = 1 },
		},
		"DEPOSIT limit tick": {
			batch:  PartialBatchFixture(),
			slot:   0,
			mutate: func(message *Message) { message.LimitTick = 1 },
		},
		"ORDER token ID": {
			batch:  FullBatchFixture(),
			slot:   2,
			mutate: func(message *Message) { message.TokenID = TokenQuote },
		},
		"ORDER deposit amount": {
			batch:  FullBatchFixture(),
			slot:   2,
			mutate: func(message *Message) { message.DepositAmount = 1 },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			test.mutate(&test.batch.Slots[test.slot])
			if err := test.batch.Validate(); err == nil {
				t.Fatal("non-canonical union unexpectedly validated")
			}
		})
	}
}

func TestNumericUpperBoundariesValidate(t *testing.T) {
	depositBatch := PartialBatchFixture()
	depositBatch.BatchIndex = MaxBatchIndex
	depositBatch.StartSequenceID = math.MaxUint64
	depositBatch.Slots[0].SequenceID = math.MaxUint64
	depositBatch.Slots[0].DepositAmount = MaxDepositAmount
	if err := depositBatch.Validate(); err != nil {
		t.Fatalf("validate maximum deposit batch: %v", err)
	}

	orderBatch := FullBatchFixture()
	orderBatch.Slots[2].BaseAmount = MaxOrderAmount
	orderBatch.Slots[2].LimitTick = MaxLimitTick
	if err := orderBatch.Validate(); err != nil {
		t.Fatalf("validate maximum order fields: %v", err)
	}
}

func TestCommitmentSplitsDigestIntoBigEndian128BitLimbs(t *testing.T) {
	commitment, err := CommitBatch(PartialBatchFixture())
	if err != nil {
		t.Fatalf("commit partial batch: %v", err)
	}
	if got, want := hex.EncodeToString(commitment.Hi[:]), hex.EncodeToString(commitment.Digest[:16]); got != want {
		t.Fatalf("high limb = %s, want %s", got, want)
	}
	if got, want := hex.EncodeToString(commitment.Lo[:]), hex.EncodeToString(commitment.Digest[16:]); got != want {
		t.Fatalf("low limb = %s, want %s", got, want)
	}
}
