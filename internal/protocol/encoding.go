package protocol

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/sha3"
)

const (
	BatchEncodingVersion = 1
	EncodedSlotSize      = 25
	EncodedBatchSize     = 32 + 8 + 8 + 1 + MaxSlots*EncodedSlotSize
)

// "FIFO_CLEAR_BATCH_V1", right-padded with zero bytes to bytes32.
var batchDomainSeparator = [32]byte{
	0x46, 0x49, 0x46, 0x4f, 0x5f, 0x43, 0x4c, 0x45,
	0x41, 0x52, 0x5f, 0x42, 0x41, 0x54, 0x43, 0x48,
	0x5f, 0x56, 0x31, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

type Commitment struct {
	Digest [32]byte
	Hi     [16]byte
	Lo     [16]byte
}

func BatchDomainSeparator() [32]byte {
	return batchDomainSeparator
}

// EncodeBatch returns the sole canonical batch encoding. Every integer is
// fixed-width, unsigned, and big-endian, matching Solidity abi.encodePacked.
func EncodeBatch(batch Batch) ([]byte, error) {
	if err := batch.Validate(); err != nil {
		return nil, fmt.Errorf("validate batch: %w", err)
	}

	encoded := make([]byte, EncodedBatchSize)
	offset := copy(encoded, batchDomainSeparator[:])
	binary.BigEndian.PutUint64(encoded[offset:offset+8], batch.BatchIndex)
	offset += 8
	binary.BigEndian.PutUint64(encoded[offset:offset+8], batch.StartSequenceID)
	offset += 8
	encoded[offset] = batch.MessageCount
	offset++

	for _, message := range batch.Slots {
		encoded[offset] = byte(message.Type)
		offset++
		binary.BigEndian.PutUint64(encoded[offset:offset+8], message.SequenceID)
		offset += 8
		encoded[offset] = byte(message.AccountID)
		offset++
		encoded[offset] = byte(message.TokenID)
		offset++
		binary.BigEndian.PutUint64(encoded[offset:offset+8], message.DepositAmount)
		offset += 8
		encoded[offset] = byte(message.Side)
		offset++
		binary.BigEndian.PutUint32(encoded[offset:offset+4], uint32(message.BaseAmount))
		offset += 4
		encoded[offset] = byte(message.LimitTick)
		offset++
	}
	if offset != EncodedBatchSize {
		panic(fmt.Sprintf("canonical encoding wrote %d bytes, want %d", offset, EncodedBatchSize))
	}
	return encoded, nil
}

func CommitBatch(batch Batch) (Commitment, error) {
	encoded, err := EncodeBatch(batch)
	if err != nil {
		return Commitment{}, err
	}
	hasher := sha3.NewLegacyKeccak256()
	_, _ = hasher.Write(encoded)
	var commitment Commitment
	copy(commitment.Digest[:], hasher.Sum(nil))
	copy(commitment.Hi[:], commitment.Digest[:16])
	copy(commitment.Lo[:], commitment.Digest[16:])
	return commitment, nil
}
