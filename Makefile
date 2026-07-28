GO ?= go
FORGE ?= forge
ARTIFACT_DIR := build/proof-spike
SOLIDITY_VERIFIER := contracts/generated/Verifier.sol
PRODUCTION_ARTIFACT_DIR := build/production-proof
PRODUCTION_SOLIDITY_VERIFIER := contracts/generated/ProductionVerifier.sol
PROTOCOL_VECTORS := testdata/protocol_vectors.json
ENGINE_VECTORS := testdata/engine_vectors.json

.PHONY: test test-engine test-primitives test-circuit test-solidity-primitives generate-vectors generate-engine-vectors proof-spike production-proof test-anvil test-production-anvil

test:
	$(GO) test ./...

test-engine:
	$(GO) test ./internal/engine/... ./internal/enginevectors

test-primitives:
	$(GO) test ./internal/protocol ./internal/state ./internal/vectors

test-circuit:
	$(GO) test ./internal/circuit ./internal/productionproof

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

production-proof:
	$(GO) run ./cmd/production-proof \
		-output $(PRODUCTION_ARTIFACT_DIR) \
		-solidity $(PRODUCTION_SOLIDITY_VERIFIER)

test-anvil: proof-spike
	./scripts/test-anvil.sh

test-production-anvil: production-proof
	./scripts/test-production-anvil.sh
