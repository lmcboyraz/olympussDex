# Solidity source directory

`make proof-spike` generates the Milestone 0 toolchain-only
`generated/Verifier.sol`. `make production-proof` separately generates the
Milestone 3 FIFO-Clear `generated/ProductionVerifier.sol`. Both verifiers are
ignored and rebuilt from pinned Go dependencies; neither is a persistent
proving-key deployment.

`src/BatchCommitmentHarness.sol` is a tracked, test-only Milestone 1 harness.
It mirrors the canonical packed batch layout so Foundry can compare Solidity
bytes and Keccak commitments with the tracked Go golden vectors. It is not the
production FIFO inbox.
