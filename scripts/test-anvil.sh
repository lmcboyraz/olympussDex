#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
artifact_directory="$repository_root/build/proof-spike"
call_data_path="$artifact_directory/calldata.json"
verifier_path="$repository_root/contracts/generated/Verifier.sol"
anvil_log="$artifact_directory/anvil.log"
anvil_port="${ANVIL_PORT:-8545}"
rpc_url="http://127.0.0.1:${anvil_port}"
anvil_private_key="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

for required_command in forge cast anvil jq; do
	if ! command -v "$required_command" >/dev/null 2>&1; then
		echo "required command not found: $required_command" >&2
		exit 1
	fi
done

if [[ ! -f "$call_data_path" || ! -f "$verifier_path" ]]; then
	echo "proof-spike artifacts are missing; run make proof-spike first" >&2
	exit 1
fi

forge build --root "$repository_root"

if cast block-number --rpc-url "$rpc_url" >/dev/null 2>&1; then
	echo "Anvil port is already in use: $anvil_port" >&2
	exit 1
fi

anvil --silent --port "$anvil_port" >"$anvil_log" 2>&1 &
anvil_pid=$!
cleanup() {
	kill "$anvil_pid" >/dev/null 2>&1 || true
	wait "$anvil_pid" >/dev/null 2>&1 || true
}
trap cleanup EXIT

for _ in {1..100}; do
	if cast block-number --rpc-url "$rpc_url" >/dev/null 2>&1; then
		break
	fi
	sleep 0.1
done
if ! cast block-number --rpc-url "$rpc_url" >/dev/null 2>&1; then
	echo "Anvil did not become ready; see $anvil_log" >&2
	exit 1
fi

deployment="$(
	forge create \
		--root "$repository_root" \
		--rpc-url "$rpc_url" \
		--private-key "$anvil_private_key" \
		--broadcast \
		--json \
		"$verifier_path:Verifier"
)"
verifier_address="$(jq -er '.deployedTo // .deployed_to' <<<"$deployment")"

proof_arguments="$(jq -er '.proof | join(",")' "$call_data_path")"
public_input_arguments="$(jq -er '[.publicInputs[].value] | join(",")' "$call_data_path")"
public_input_count="$(jq -er '.publicInputs | length' "$call_data_path")"
verifier_signature="verifyProof(uint256[8],uint256[$public_input_count])"

cast call \
	--rpc-url "$rpc_url" \
	"$verifier_address" \
	"$verifier_signature" \
	"[$proof_arguments]" \
	"[$public_input_arguments]" \
	>/dev/null
echo "Anvil valid proof verification: PASS"

mutated_public_input_arguments="$(
	jq -er '
		.publicInputs[0].value = ((.publicInputs[0].value | tonumber) + 1 | tostring)
		| [.publicInputs[].value]
		| join(",")
	' "$call_data_path"
)"
if cast call \
	--rpc-url "$rpc_url" \
	"$verifier_address" \
	"$verifier_signature" \
	"[$proof_arguments]" \
	"[$mutated_public_input_arguments]" \
	>/dev/null 2>&1
then
	echo "mutated public input unexpectedly verified" >&2
	exit 1
fi
echo "Anvil mutated public input rejection: PASS"
echo "Verifier deployed at: $verifier_address"
