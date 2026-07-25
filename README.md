# FIFO-Clear proof toolchain spike

This repository currently contains only Milestone 0: a minimal BN254 Groth16
vertical spike proving that gnark-generated proofs can be verified both in Go
and by a generated Solidity verifier on Anvil. It does not implement the final
FIFO-Clear circuit or any production protocol logic.

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
make proof-spike
make test-anvil
```

`make proof-spike` compiles the circuit, runs Groth16 setup, creates a witness
and proof, verifies the proof in Go, and generates the Solidity verifier plus
the exact calldata consumed by `make test-anvil`.

`make test-anvil` builds and deploys that verifier to a temporary local Anvil
node. It checks that the generated proof succeeds and that changing
`batchIndex` while keeping the same proof is rejected.

All R1CS, keys, witnesses, proofs, generated Solidity, calldata, Foundry output,
and Anvil logs are disposable ignored artifacts.

## Spike constraint

The canonical public-input order is defined only in
`internal/publicinputs/public_inputs.go`. The spike constrains input 0 to the
square of a private secret. Each input from 1 through 26 must equal the previous
public input plus its own index. This binds every public input without
implementing any Milestone 1 protocol logic.
