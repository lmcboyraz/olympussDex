// SPDX-License-Identifier: MIT
pragma solidity 0.8.35;

import {BatchCommitmentHarness} from "../../contracts/src/BatchCommitmentHarness.sol";

interface Vm {
    function readFile(string calldata path) external view returns (string memory);
    function parseJsonBytes(string calldata json, string calldata key) external pure returns (bytes memory);
    function parseJsonUint(string calldata json, string calldata key) external pure returns (uint256);
    function toString(uint256 value) external pure returns (string memory);
}

contract BatchCommitmentHarnessTest {
    Vm private constant vm = Vm(address(uint160(uint256(keccak256("hevm cheat code")))));

    BatchCommitmentHarness private harness;

    function setUp() external {
        harness = new BatchCommitmentHarness();
    }

    function testFullBatchMatchesGoGoldenVector() external view {
        _assertGoldenVector(".batches.full");
    }

    function testPartialBatchMatchesGoGoldenVector() external view {
        _assertGoldenVector(".batches.partial");
    }

    function _assertGoldenVector(string memory vectorPath) private view {
        string memory golden = vm.readFile("testdata/protocol_vectors.json");
        bytes32 expectedDomain = _asBytes32(vm.parseJsonBytes(golden, ".domains.batchDomainSeparator"));
        require(harness.BATCH_DOMAIN_SEPARATOR() == expectedDomain, "batch domain mismatch");

        uint64 batchIndex = uint64(vm.parseJsonUint(golden, string.concat(vectorPath, ".batchIndex")));
        uint64 startSequenceId = uint64(vm.parseJsonUint(golden, string.concat(vectorPath, ".startSequenceId")));
        uint8 messageCount = uint8(vm.parseJsonUint(golden, string.concat(vectorPath, ".messageCount")));
        BatchCommitmentHarness.Message[8] memory slots;
        for (uint256 index = 0; index < slots.length; index++) {
            string memory slotPath = string.concat(vectorPath, ".slots[", vm.toString(index), "]");
            slots[index] = BatchCommitmentHarness.Message({
                messageType: uint8(vm.parseJsonUint(golden, string.concat(slotPath, ".messageType"))),
                sequenceId: uint64(vm.parseJsonUint(golden, string.concat(slotPath, ".sequenceId"))),
                accountId: uint8(vm.parseJsonUint(golden, string.concat(slotPath, ".accountId"))),
                tokenId: uint8(vm.parseJsonUint(golden, string.concat(slotPath, ".tokenId"))),
                depositAmount: uint64(vm.parseJsonUint(golden, string.concat(slotPath, ".depositAmount"))),
                side: uint8(vm.parseJsonUint(golden, string.concat(slotPath, ".side"))),
                baseAmount: uint32(vm.parseJsonUint(golden, string.concat(slotPath, ".baseAmount"))),
                limitTick: uint8(vm.parseJsonUint(golden, string.concat(slotPath, ".limitTick")))
            });
        }

        bytes memory expectedEncoding = vm.parseJsonBytes(golden, string.concat(vectorPath, ".canonicalBytes"));
        bytes32 expectedDigest = _asBytes32(vm.parseJsonBytes(golden, string.concat(vectorPath, ".keccakDigest")));
        bytes16 expectedHi = _asBytes16(vm.parseJsonBytes(golden, string.concat(vectorPath, ".commitmentHi")));
        bytes16 expectedLo = _asBytes16(vm.parseJsonBytes(golden, string.concat(vectorPath, ".commitmentLo")));

        bytes memory actualEncoding = harness.encodeBatch(batchIndex, startSequenceId, messageCount, slots);
        require(
            actualEncoding.length == expectedEncoding.length
                && keccak256(actualEncoding) == keccak256(expectedEncoding),
            "canonical encoding mismatch"
        );

        (bytes32 actualDigest, bytes16 actualHi, bytes16 actualLo) =
            harness.commitBatch(batchIndex, startSequenceId, messageCount, slots);
        require(actualDigest == expectedDigest, "Keccak digest mismatch");
        require(actualHi == expectedHi, "high limb mismatch");
        require(actualLo == expectedLo, "low limb mismatch");
    }

    function _asBytes32(bytes memory value) private pure returns (bytes32 result) {
        return _loadWord(value, 32);
    }

    function _asBytes16(bytes memory value) private pure returns (bytes16 result) {
        return bytes16(_loadWord(value, 16));
    }

    function _loadWord(bytes memory value, uint256 expectedLength) private pure returns (bytes32 result) {
        require(value.length == expectedLength, "unexpected byte length");
        assembly ("memory-safe") {
            result := mload(add(value, 0x20))
        }
    }
}
