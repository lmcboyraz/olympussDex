// SPDX-License-Identifier: MIT
pragma solidity 0.8.35;

/// @notice Test-only mirror of the canonical Go batch byte layout.
contract BatchCommitmentHarness {
    bytes32 public constant BATCH_DOMAIN_SEPARATOR =
        hex"4649464f5f434c4541525f42415443485f563100000000000000000000000000";

    struct Message {
        uint8 messageType;
        uint64 sequenceId;
        uint8 accountId;
        uint8 tokenId;
        uint64 depositAmount;
        uint8 side;
        uint32 baseAmount;
        uint8 limitTick;
    }

    function encodeBatch(uint64 batchIndex, uint64 startSequenceId, uint8 messageCount, Message[8] calldata slots)
        public
        pure
        returns (bytes memory encoded)
    {
        encoded = abi.encodePacked(BATCH_DOMAIN_SEPARATOR, batchIndex, startSequenceId, messageCount);
        for (uint256 index = 0; index < slots.length; index++) {
            Message calldata message = slots[index];
            encoded = bytes.concat(
                encoded,
                abi.encodePacked(
                    message.messageType,
                    message.sequenceId,
                    message.accountId,
                    message.tokenId,
                    message.depositAmount,
                    message.side,
                    message.baseAmount,
                    message.limitTick
                )
            );
        }
    }

    function commitBatch(uint64 batchIndex, uint64 startSequenceId, uint8 messageCount, Message[8] calldata slots)
        external
        pure
        returns (bytes32 digest, bytes16 commitmentHi, bytes16 commitmentLo)
    {
        digest = keccak256(encodeBatch(batchIndex, startSequenceId, messageCount, slots));
        // Deliberately select digest[0..15] as the high 128-bit limb.
        // forge-lint: disable-next-line(unsafe-typecast)
        commitmentHi = bytes16(digest);
        // Shift digest[16..31] left before deliberately selecting 16 bytes.
        // forge-lint: disable-next-line(unsafe-typecast)
        commitmentLo = bytes16(digest << 128);
    }
}
