# FIFO-Clear protocol primitives

The repository contains the Milestone 0 BN254 Groth16 toolchain spike and the
Milestone 1 canonical protocol primitives. Milestone 1 defines message
validation, fixed-width Keccak batch commitments, a Poseidon2 Merkle state, and
the deterministic genesis fixture. It does not implement clearing, settlement,
production inbox contracts, or the final FIFO-Clear circuit.

## Pinned toolchain

- Go 1.26.1 (`go.mod`, `toolchain`, and `.go-version`)
- gnark v0.14.0
- gnark-crypto v0.19.0
- Foundry v1.7.1 (`.foundry-version`)
- Solidity 0.8.35 (`foundry.toml`)

Install the pinned Go and Foundry releases and ensure `go`, `forge`, `cast`,
`anvil`, and `jq` are available on `PATH`.

## Commands

```text
make test
make test-primitives
make test-solidity-primitives
make generate-vectors
make proof-spike
make test-anvil
```

`make test` runs every Go test, including the unchanged proof spike.
`make test-primitives` runs only protocol, state, and golden-vector Go tests.
`make test-solidity-primitives` checks the Solidity test harness against the
tracked Go vectors. `make generate-vectors` is the only command that rewrites
`testdata/protocol_vectors.json`; normal tests only read and compare it.

`make proof-spike` compiles the circuit, runs Groth16 setup, creates a witness
and proof, verifies the proof in Go, and generates the Solidity verifier plus
the exact calldata consumed by `make test-anvil`.

`make test-anvil` builds and deploys that verifier to a temporary local Anvil
node. It checks that the generated proof succeeds and that changing
`batchIndex` while keeping the same proof is rejected.

All R1CS, keys, witnesses, proofs, generated Solidity, calldata, Foundry output,
and Anvil logs are disposable ignored artifacts.

## Canonical batch encoding

All unsigned integers use fixed-width big-endian bytes, matching Solidity
`abi.encodePacked`. Every encoding is exactly 249 bytes:

```text
bytes32 batchDomainSeparator
uint64  batchIndex
uint64  startSequenceId
uint8   messageCount
8 × (
  uint8  messageType
  uint64 sequenceId
  uint8  accountId
  uint8  tokenId
  uint64 depositAmount
  uint8  side
  uint32 baseAmount
  uint8  limitTick
)
```

The commitment is `keccak256(canonicalEncoding)`; its first and last 16 bytes
are respectively `commitmentHi` and `commitmentLo`. The canonical Go definition
lives in `internal/protocol`.

## Poseidon2 state

The state is a depth-4 tree with eight account leaves, one AMM leaf, one
metadata leaf, and six index-bound empty leaves. Hashing uses
`bn254/fr/poseidon2.NewMerkleDamgardHasher` with explicit v1 field domains:
account `257`, AMM `258`, metadata `259`, empty `260`, and internal node `261`.
Internal-node preimages also include their zero-based tree level.

The tracked vectors under `testdata/` contain every genesis leaf, the genesis
root, a Merkle proof, and full/partial batch commitment fixtures.

## Milestone 0 spike constraint

The canonical public-input order is defined only in
`internal/publicinputs/public_inputs.go`. The spike constrains input 0 to the
square of a private secret. Each input from 1 through 26 must equal the previous
public input plus its own index. This binds every public input without
implementing protocol logic.
