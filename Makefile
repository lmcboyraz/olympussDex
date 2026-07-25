GO ?= go
FORGE ?= forge
ARTIFACT_DIR := build/proof-spike
SOLIDITY_VERIFIER := contracts/generated/Verifier.sol
PROTOCOL_VECTORS := testdata/protocol_vectors.json
ENGINE_VECTORS := testdata/engine_vectors.json

.PHONY: test test-engine test-primitives test-solidity-primitives generate-vectors generate-engine-vectors proof-spike test-anvil

test:
	$(GO) test ./...

test-engine:
	$(GO) test ./internal/engine/... ./internal/enginevectors

test-primitives:
	$(GO) test ./internal/protocol ./internal/state ./internal/vectors

test-solidity-primitives:
	$(FORGE) test --match-path 'test/solidity/*'

generate-vectors:
	$(GO) run ./cmd/generate-vectors -output $(PROTOCOL_VECTORS)

generate-engine-vectors:
	$(GO) run ./cmd/generate-engine-vectors -output $(ENGINE_VECTORS)

proof-spike:
	$(GO) run ./cmd/proof-spike \
		-output $(ARTIFACT_DIR) \
		-solidity $(SOLIDITY_VERIFIER)

test-anvil: proof-spike
	./scripts/test-anvil.sh
