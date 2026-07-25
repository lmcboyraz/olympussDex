// Package enginevectors builds the tracked, exact Milestone 2 showcase
// vectors. Normal tests compare against the tracked file and never rewrite it.
package enginevectors

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cemilboyraz/oly2/internal/engine"
	"github.com/cemilboyraz/oly2/internal/protocol"
	"github.com/cemilboyraz/oly2/internal/publicinputs"
	"github.com/cemilboyraz/oly2/internal/state"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

const GoldenVectorSchemaVersion = 1

type FieldElement struct {
	Decimal string `json:"decimal"`
	Hex     string `json:"hex"`
}

type AccountVector struct {
	AccountID    int    `json:"accountId"`
	BaseBalance  uint64 `json:"baseBalance"`
	QuoteBalance uint64 `json:"quoteBalance"`
}

type AMMVector struct {
	BaseReserve  uint64 `json:"baseReserve"`
	QuoteReserve uint64 `json:"quoteReserve"`
}

type MetadataVector struct {
	ProcessedBatchCount   uint64 `json:"processedBatchCount"`
	ProcessedMessageCount uint64 `json:"processedMessageCount"`
}

type StateVector struct {
	Accounts []AccountVector `json:"accounts"`
	AMM      AMMVector       `json:"amm"`
	Metadata MetadataVector  `json:"metadata"`
}

type MessageVector struct {
	Slot          int    `json:"slot"`
	MessageType   uint8  `json:"messageType"`
	SequenceID    uint64 `json:"sequenceId"`
	AccountID     uint64 `json:"accountId"`
	TokenID       uint8  `json:"tokenId"`
	DepositAmount uint64 `json:"depositAmount"`
	Side          uint8  `json:"side"`
	BaseAmount    uint64 `json:"baseAmount"`
	LimitTick     uint64 `json:"limitTick"`
}

type BatchInputVector struct {
	BatchIndex      uint64          `json:"batchIndex"`
	StartSequenceID uint64          `json:"startSequenceId"`
	MessageCount    uint8           `json:"messageCount"`
	Slots           []MessageVector `json:"slots"`
}

type TraceVector struct {
	FundingStatus         [protocol.MaxSlots]uint8  `json:"fundingStatus"`
	FilledBaseAmount      [protocol.MaxSlots]uint64 `json:"filledBaseAmount"`
	ClearingTick          uint8                     `json:"clearingTick"`
	Demand                uint64                    `json:"demand"`
	Supply                uint64                    `json:"supply"`
	InternalMatchedBase   uint64                    `json:"internalMatchedBase"`
	RequestedResidualBase uint64                    `json:"requestedResidualBase"`
	AMMResidualDirection  uint8                     `json:"ammResidualDirection"`
	AMMFilledBase         uint64                    `json:"ammFilledBase"`
	UnfilledResidualBase  uint64                    `json:"unfilledResidualBase"`
}

type PublicInputVector struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Decimal string `json:"decimal"`
}

type BatchOutputVector struct {
	OldStateRoot      FieldElement        `json:"oldStateRoot"`
	NewStateRoot      FieldElement        `json:"newStateRoot"`
	BatchCommitment   string              `json:"batchCommitment"`
	BatchCommitmentHi string              `json:"batchCommitmentHi"`
	BatchCommitmentLo string              `json:"batchCommitmentLo"`
	Trace             TraceVector         `json:"trace"`
	PostState         StateVector         `json:"postState"`
	PublicInputs      []PublicInputVector `json:"publicInputs"`
}

type BatchVector struct {
	Name   string            `json:"name"`
	Input  BatchInputVector  `json:"input"`
	Output BatchOutputVector `json:"output"`
}

type GoldenVectors struct {
	Version int           `json:"version"`
	Batches []BatchVector `json:"batches"`
}

func Build() (GoldenVectors, error) {
	pre := state.GenesisFixture()
	batches := []struct {
		name  string
		batch protocol.Batch
	}{
		{name: "showcaseBatch0ExcessBuy", batch: engine.ShowcaseBatch0()},
		{name: "showcaseBatch1ExcessSell", batch: engine.ShowcaseBatch1()},
	}

	result := GoldenVectors{
		Version: GoldenVectorSchemaVersion,
		Batches: make([]BatchVector, len(batches)),
	}
	for index, item := range batches {
		execution, err := engine.Execute(pre, item.batch)
		if err != nil {
			return GoldenVectors{}, fmt.Errorf("execute %s: %w", item.name, err)
		}
		result.Batches[index] = batchVector(item.name, item.batch, execution)
		pre = execution.PostState
	}
	return result, nil
}

func Marshal() ([]byte, error) {
	vectors, err := Build()
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(vectors, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal engine golden vectors: %w", err)
	}
	return append(encoded, '\n'), nil
}

func Write(path string) error {
	encoded, err := Marshal()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create engine vector directory: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write engine golden vectors: %w", err)
	}
	return nil
}

func batchVector(name string, batch protocol.Batch, result engine.Result) BatchVector {
	input := BatchInputVector{
		BatchIndex:      batch.BatchIndex,
		StartSequenceID: batch.StartSequenceID,
		MessageCount:    batch.MessageCount,
		Slots:           make([]MessageVector, protocol.MaxSlots),
	}
	for slot, message := range batch.Slots {
		input.Slots[slot] = MessageVector{
			Slot:          slot,
			MessageType:   uint8(message.Type),
			SequenceID:    message.SequenceID,
			AccountID:     message.AccountID,
			TokenID:       uint8(message.TokenID),
			DepositAmount: message.DepositAmount,
			Side:          uint8(message.Side),
			BaseAmount:    message.BaseAmount,
			LimitTick:     message.LimitTick,
		}
	}

	var statuses [protocol.MaxSlots]uint8
	for slot, status := range result.FundingStatus {
		statuses[slot] = uint8(status)
	}
	publicValues := result.PublicInputs()
	publicNames := publicinputs.Names()
	publicVectors := make([]PublicInputVector, publicinputs.Count)
	for index := range publicVectors {
		publicVectors[index] = PublicInputVector{
			Index:   index,
			Name:    publicNames[index],
			Decimal: publicValues[index].String(),
		}
	}

	return BatchVector{
		Name:  name,
		Input: input,
		Output: BatchOutputVector{
			OldStateRoot:      fieldElement(result.OldStateRoot),
			NewStateRoot:      fieldElement(result.NewStateRoot),
			BatchCommitment:   hexBytes(result.BatchCommitment.Digest[:]),
			BatchCommitmentHi: hexBytes(result.BatchCommitment.Hi[:]),
			BatchCommitmentLo: hexBytes(result.BatchCommitment.Lo[:]),
			Trace: TraceVector{
				FundingStatus:         statuses,
				FilledBaseAmount:      result.FilledBaseAmount,
				ClearingTick:          result.ClearingTick,
				Demand:                result.Demand,
				Supply:                result.Supply,
				InternalMatchedBase:   result.InternalMatchedBase,
				RequestedResidualBase: result.RequestedResidualBase,
				AMMResidualDirection:  uint8(result.AMMResidualDirection),
				AMMFilledBase:         result.AMMFilledBase,
				UnfilledResidualBase:  result.UnfilledResidualBase,
			},
			PostState:    stateVector(result.PostState),
			PublicInputs: publicVectors,
		},
	}
}

func stateVector(value state.State) StateVector {
	accounts := make([]AccountVector, protocol.AccountLeafCount)
	for index, account := range value.Accounts {
		accounts[index] = AccountVector{
			AccountID:    index,
			BaseBalance:  account.BaseBalance,
			QuoteBalance: account.QuoteBalance,
		}
	}
	return StateVector{
		Accounts: accounts,
		AMM: AMMVector{
			BaseReserve:  value.AMM.BaseReserve,
			QuoteReserve: value.AMM.QuoteReserve,
		},
		Metadata: MetadataVector{
			ProcessedBatchCount:   value.Metadata.ProcessedBatchCount,
			ProcessedMessageCount: value.Metadata.ProcessedMessageCount,
		},
	}
}

func fieldElement(element fr.Element) FieldElement {
	bytes := element.Bytes()
	return FieldElement{
		Decimal: element.String(),
		Hex:     hexBytes(bytes[:]),
	}
}

func hexBytes(value []byte) string {
	return "0x" + hex.EncodeToString(value)
}
