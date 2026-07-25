# Solidity source directory

`make proof-spike` generates `generated/Verifier.sol` here. The generated
verifier is intentionally ignored by Git and is rebuilt from the pinned Go
dependencies whenever the spike runs.

`src/BatchCommitmentHarness.sol` is a tracked, test-only Milestone 1 harness.
It mirrors the canonical packed batch layout so Foundry can compare Solidity
bytes and Keccak commitments with the tracked Go golden vectors. It is not the
production FIFO inbox.
