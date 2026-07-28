# FIFO-Clear protocol primitives

The repository contains the Milestone 0 BN254 Groth16 toolchain spike,
Milestone 1 canonical protocol primitives, and the Milestone 2 deterministic Go
reference engine. The reference engine implements sequential funding,
FIFO-Clear candidate selection, constant-product AMM residual execution,
strict FIFO allocation, settlement, and conservation checks. It is not the
final FIFO-Clear circuit or a production inbox/artifact pipeline.

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
make test-engine
make test-primitives
make test-solidity-primitives
make generate-vectors
make generate-engine-vectors
make proof-spike
make test-anvil
```

`make test` runs every Go test, including the unchanged proof spike.
`make test-engine` runs the reference engine, AMM, clearing, and tracked
showcase-vector tests.
`make test-primitives` runs only protocol, state, and golden-vector Go tests.
`make test-solidity-primitives` checks the Solidity test harness against the
tracked Go vectors. `make generate-vectors` is the only command that rewrites
`testdata/protocol_vectors.json`; normal tests only read and compare it.
Likewise, `make generate-engine-vectors` is the only command that rewrites
`testdata/engine_vectors.json`.

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

## Reference engine

`internal/engine.Execute` accepts a pre-state and a canonical protocol batch.
It validates batch and metadata chaining, processes deposits and reservations
in slot order, evaluates only active-order limit ticks against the pre-batch
AMM, allocates the winning liquidity in strict FIFO order, settles/refunds all
reservations, updates metadata, and verifies token conservation. Its result
maps directly to the canonical 27-public-input order.

Every persistent account balance and AMM reserve is a `uint64`. In addition,
the global aggregate liability for each token—eight account balances plus the
AMM reserve—is explicitly capped at `uint64` maximum. The engine calculates
pre-state liabilities, batch deposits, expected liabilities, and post-state
liabilities with unsigned 128-bit arithmetic before enforcing that cap.

The AMM residual search is logarithmic and all reserve-product comparisons use
unsigned 128-bit arithmetic. The exact two-batch showcase states, roots,
commitments, traces, and public inputs are tracked in
`testdata/engine_vectors.json`. Normal tests only read and compare that file;
only `make generate-engine-vectors` rewrites it. Hand-authored showcase
assertions independently pin the expected balances, AMM reserves, metadata,
roots, funding statuses, and fills.

## Milestone 0 spike constraint

The canonical public-input order is defined only in
`internal/publicinputs/public_inputs.go`. The spike constrains input 0 to the
square of a private secret. Each input from 1 through 26 must equal the previous
public input plus its own index. This binds every public input without
implementing protocol logic.
