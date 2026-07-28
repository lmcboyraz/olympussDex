package circuit

import (
	"fmt"

	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/sha3"
	"github.com/consensys/gnark/std/math/uints"
)

var circuitBatchDomainSeparator = protocol.BatchDomainSeparator()

type messageFlags struct {
	real      frontend.Variable
	deposit   frontend.Variable
	order     frontend.Variable
	buy       frontend.Variable
	sell      frontend.Variable
	baseToken frontend.Variable
}

// ConstrainBatchBinding validates the private canonical batch, recreates its
// exact 249-byte M1 encoding and Legacy Keccak-256 commitment, and binds the
// batch header and digest halves to public values.
func ConstrainBatchBinding(
	api frontend.API,
	batch BatchWitness,
	publicBatchIndex frontend.Variable,
	publicMessageCount frontend.Variable,
	publicCommitmentHigh frontend.Variable,
	publicCommitmentLow frontend.Variable,
) error {
	_, err := constrainBatch(api, batch)
	if err != nil {
		return err
	}
	g := NewGadgets(api)
	g.AssertUnsigned(publicBatchIndex, 61)
	g.AssertUnsigned(publicMessageCount, 4)
	g.AssertUnsigned(publicCommitmentHigh, 128)
	g.AssertUnsigned(publicCommitmentLow, 128)
	api.AssertIsEqual(batch.BatchIndex, publicBatchIndex)
	api.AssertIsEqual(batch.MessageCount, publicMessageCount)

	encoded, err := encodeBatch(api, batch)
	if err != nil {
		return err
	}
	hasher, err := sha3.NewLegacyKeccak256(api)
	if err != nil {
		return fmt.Errorf("create Legacy Keccak-256 circuit: %w", err)
	}
	hasher.Write(encoded)
	digest := hasher.Sum()
	bytesAPI, err := uints.NewBytes(api)
	if err != nil {
		return fmt.Errorf("create byte API: %w", err)
	}
	high := packBigEndian(api, bytesAPI, digest[:16])
	low := packBigEndian(api, bytesAPI, digest[16:])
	api.AssertIsEqual(high, publicCommitmentHigh)
	api.AssertIsEqual(low, publicCommitmentLow)
	return nil
}

func constrainBatch(api frontend.API, batch BatchWitness) ([protocol.MaxSlots]messageFlags, error) {
	g := NewGadgets(api)
	g.AssertUnsigned(batch.BatchIndex, 61)
	g.AssertUnsigned(batch.StartSequenceID, 64)
	g.AssertUnsigned(batch.MessageCount, 4)
	api.AssertIsEqual(g.IsLess(0, batch.MessageCount, 4), 1)
	api.AssertIsEqual(g.IsLessOrEqual(batch.MessageCount, protocol.MaxSlots, 4), 1)

	var flags [protocol.MaxSlots]messageFlags
	for slot, message := range batch.Slots {
		g.AssertUnsigned(message.Type, 2)
		g.AssertEnum(message.Type, 0, 1, 2)
		g.AssertUnsigned(message.SequenceID, 64)
		g.AssertUnsigned(message.AccountID, 3)
		g.AssertUnsigned(message.TokenID, 1)
		g.AssertUnsigned(message.DepositAmount, 64)
		g.AssertUnsigned(message.Side, 1)
		g.AssertUnsigned(message.BaseAmount, 32)
		g.AssertUnsigned(message.LimitTick, 8)

		empty := g.IsEqual(message.Type, uint64(protocol.MessageTypeEmpty))
		deposit := g.IsEqual(message.Type, uint64(protocol.MessageTypeDeposit))
		order := g.IsEqual(message.Type, uint64(protocol.MessageTypeOrder))
		real := g.IsLess(slot, batch.MessageCount, 4)
		api.AssertIsEqual(api.Add(empty, deposit, order), 1)
		api.AssertIsEqual(real, api.Sub(1, empty))

		g.AssertEqualWhen(real, message.SequenceID, api.Add(batch.StartSequenceID, slot))
		g.AssertEqualWhen(empty, message.SequenceID, 0)
		g.AssertEqualWhen(empty, message.AccountID, 0)
		g.AssertEqualWhen(empty, message.TokenID, 0)
		g.AssertEqualWhen(empty, message.DepositAmount, 0)
		g.AssertEqualWhen(empty, message.Side, 0)
		g.AssertEqualWhen(empty, message.BaseAmount, 0)
		g.AssertEqualWhen(empty, message.LimitTick, 0)

		api.AssertIsEqual(api.Mul(deposit, api.IsZero(message.DepositAmount)), 0)
		g.AssertEqualWhen(deposit, message.Side, 0)
		g.AssertEqualWhen(deposit, message.BaseAmount, 0)
		g.AssertEqualWhen(deposit, message.LimitTick, 0)

		api.AssertIsEqual(api.Mul(order, api.IsZero(message.BaseAmount)), 0)
		g.AssertEqualWhen(order, message.TokenID, 0)
		g.AssertEqualWhen(order, message.DepositAmount, 0)

		flags[slot] = messageFlags{
			real:      real,
			deposit:   deposit,
			order:     order,
			buy:       api.Mul(order, api.Sub(1, message.Side)),
			sell:      api.Mul(order, message.Side),
			baseToken: api.Mul(deposit, api.Sub(1, message.TokenID)),
		}
	}
	return flags, nil
}

func encodeBatch(api frontend.API, batch BatchWitness) ([]uints.U8, error) {
	bytesAPI, err := uints.NewBytes(api)
	if err != nil {
		return nil, fmt.Errorf("create byte API: %w", err)
	}
	encoded := make([]uints.U8, 0, protocol.EncodedBatchSize)
	for _, value := range circuitBatchDomainSeparator {
		encoded = append(encoded, uints.NewU8(value))
	}
	encoded = append(encoded, toBytesBigEndian(api, bytesAPI, batch.BatchIndex, 8)...)
	encoded = append(encoded, toBytesBigEndian(api, bytesAPI, batch.StartSequenceID, 8)...)
	encoded = append(encoded, bytesAPI.ValueOf(batch.MessageCount))
	for _, message := range batch.Slots {
		encoded = append(encoded, bytesAPI.ValueOf(message.Type))
		encoded = append(encoded, toBytesBigEndian(api, bytesAPI, message.SequenceID, 8)...)
		encoded = append(encoded, bytesAPI.ValueOf(message.AccountID))
		encoded = append(encoded, bytesAPI.ValueOf(message.TokenID))
		encoded = append(encoded, toBytesBigEndian(api, bytesAPI, message.DepositAmount, 8)...)
		encoded = append(encoded, bytesAPI.ValueOf(message.Side))
		encoded = append(encoded, toBytesBigEndian(api, bytesAPI, message.BaseAmount, 4)...)
		encoded = append(encoded, bytesAPI.ValueOf(message.LimitTick))
	}
	if len(encoded) != protocol.EncodedBatchSize {
		return nil, fmt.Errorf(
			"circuit batch encoding size = %d, want %d",
			len(encoded),
			protocol.EncodedBatchSize,
		)
	}
	return encoded, nil
}

func toBytesBigEndian(
	api frontend.API,
	bytesAPI *uints.Bytes,
	value frontend.Variable,
	byteCount int,
) []uints.U8 {
	binary := api.ToBinary(value, byteCount*8)
	result := make([]uints.U8, byteCount)
	for outputIndex := range result {
		littleIndex := byteCount - outputIndex - 1
		start := littleIndex * 8
		byteValue := api.FromBinary(binary[start : start+8]...)
		result[outputIndex] = bytesAPI.ValueOf(byteValue)
	}
	return result
}

func packBigEndian(
	api frontend.API,
	bytesAPI *uints.Bytes,
	values []uints.U8,
) frontend.Variable {
	result := frontend.Variable(0)
	for _, value := range values {
		result = api.Add(api.Mul(result, 256), bytesAPI.Value(value))
	}
	return result
}
