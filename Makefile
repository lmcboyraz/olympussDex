GO ?= go
ARTIFACT_DIR := build/proof-spike
SOLIDITY_VERIFIER := contracts/generated/Verifier.sol

.PHONY: test proof-spike test-anvil

test:
	$(GO) test ./...

proof-spike:
	$(GO) run ./cmd/proof-spike \
		-output $(ARTIFACT_DIR) \
		-solidity $(SOLIDITY_VERIFIER)

test-anvil: proof-spike
	./scripts/test-anvil.sh
