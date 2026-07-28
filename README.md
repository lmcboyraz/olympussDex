# FIFO-Clear production circuit

The repository contains the Milestone 0 BN254 Groth16 toolchain spike,
Milestone 1 canonical protocol primitives, and the Milestone 2 deterministic Go
reference engine. The reference engine implements sequential funding,
FIFO-Clear candidate selection, constant-product AMM residual execution,
strict FIFO allocation, settlement, and conservation checks. Milestone 3 adds
the production gnark circuit that independently constrains the same canonical
transition. Production inbox, deployment, and persistent proving-key
management remain out of scope.

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
make test-circuit
make test-solidity-primitives
make generate-vectors
make generate-engine-vectors
make proof-spike
make production-proof
make test-anvil
make test-production-anvil
```

`make test` runs every Go test, including the unchanged proof spike and the
production proof pipeline.
`make test-engine` runs the reference engine, AMM, clearing, and tracked
showcase-vector tests.
`make test-primitives` runs only protocol, state, and golden-vector Go tests.
`make test-circuit` runs the production constraint, adversarial,
differential, schema, Groth16, and Solidity-ABI tests.
`make test-solidity-primitives` checks the Solidity test harness against the
tracked Go vectors. `make generate-vectors` is the only command that rewrites
`testdata/protocol_vectors.json`; normal tests only read and compare it.
Likewise, `make generate-engine-vectors` is the only command that rewrites
`testdata/engine_vectors.json`.

`make proof-spike` retains the Milestone 0 toolchain-only circuit and commands.
It does not implement FIFO-Clear. `make production-proof` compiles the
Milestone 3 circuit, runs a disposable Groth16 setup, creates the showcase
witness and proof, verifies the proof plus a tampered-public-input rejection in
Go, and generates `ProductionVerifier.sol` with commitment-aware calldata.

`make test-production-anvil` deploys the generated production verifier to a
temporary Anvil node. It accepts the valid proof, then independently mutates
each of the 27 public inputs and requires every call to revert. `make
test-anvil` remains the smaller Milestone 0 smoke test.

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

## Milestone 3 production constraint

The production circuit lives in `internal/circuit`. Its explicit private
witness is exactly the 20-field pre-state and 67-field canonical batch. It
range-checks every 8/32/61/64/128-bit value; recreates the depth-4 Poseidon2
state tree and 249-byte Legacy Keccak commitment; applies deposits and funding
reservations in slot order; proves the maximum constant-product AMM residual;
selects `F`, `M`, spot-distance, and lowest-tick winners; allocates strict FIFO
fills; settles/refunds exact-price reservations; enforces global liability
caps and conservation; advances metadata; and recreates the post-state root.

The AMM residual hint supplies only a derived candidate value. The circuit
constrains its bounds, reserve arithmetic, `postK >= preK`, and maximality by
requiring the next residual either to violate reserve arithmetic or
`postK >= preK`.

All 27 public inputs are held in one public array and interpreted exclusively
through `internal/publicinputs/public_inputs.go`. Schema tests require exactly
27 public fields and exactly 87 explicit private fields. Constraint and input
counts are printed by `make production-proof`.

## Milestone 0 spike

The canonical public-input order is defined only in
`internal/publicinputs/public_inputs.go`. The spike constrains input 0 to the
square of a private secret. Each input from 1 through 26 must equal the previous
public input plus its own index. This binds every public input without
implementing protocol logic; it is deliberately separate from the production
FIFO-Clear circuit and command.
