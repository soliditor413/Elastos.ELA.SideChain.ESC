// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/**
 * @title Blacklist with expiring entries and 2/3 committee actions
 * @notice Committee members can add addresses with a default expiry.
 *         Upgrading to permanent lock or removing an address requires
 *         approvals from at least two thirds of the committee.
 */
contract Blacklist {
    uint256 public constant CR_COMMITTEE_SIZE = 12;
    uint256 public constant THRESHOLD = (2 * CR_COMMITTEE_SIZE + 2) / 3;
    enum Action {
        Remove,
        MakePermanent
    }

    struct Entry {
        uint64 addedAt;
        bool permanent;
        bool exists;
    }

    struct Proposal {
        Action action;
        address target;
        uint64 approvals;
        bool executed;
    }

    uint64 public immutable defaultDuration;
    uint256 public proposalCount;

    mapping(address => Entry) private entries;
    mapping(uint256 => Proposal) public proposals;
    mapping(uint256 => mapping(bytes => bool)) public proposalVotes;
    uint256 public dataNonce;

    event Blacklisted(address indexed account, uint256 expiresAt);
    event BlacklistPermanent(address indexed account);
    event BlacklistRemoved(address indexed account);
    event ProposalCreated(
        uint256 indexed proposalId,
        Action action,
        address indexed target,
        bytes indexed crPublicKey
    );
    event ProposalApproved(
        uint256 indexed proposalId,
        bytes indexed crPublicKey,
        uint64 approvals
    );

    function verifySignature(bytes calldata crPublicKey, bytes memory message, bytes calldata signature) public view returns (bool) {
      require(crPublicKey.length == 33, "NoCompress");
      require(signature.length == 65, "Invalid signature");
      require(isCurrentCRMember(crPublicKey), "Not current CR member");
        address method = address(1009);
        bytes32 digest = sha256(message);
        bytes memory data = abi.encodePacked(
            crPublicKey,
            digest,
            signature
        );
        (bool success, bytes memory result) = method.staticcall(data);
        require(success, "call failed");
        uint256 result_v = uint256(bytes32(result));
        return result_v == 1;
    }

    function isCurrentCRMember(bytes calldata crPublicKey) public view returns (bool) {
        address method = address(1000);
        (bool success, bytes memory result) = method.staticcall("0x00");
        if (!success) {
            return false; // empty array
        }
        if (result.length == 0 || result.length % 32 != 0) {
            return false; // not ABI-encoded array, ignore
        }
        bytes32 publicKeyHash = keccak256(crPublicKey);
        uint256 len = result.length / 32;
        for (uint256 i = 0; i < len; i++) {
            bytes32 slot;
            // solhint-disable-next-line no-inline-assembly
            assembly {
                slot := mload(add(add(result, 0x20), mul(i, 0x20)))
            }
           if (slot == publicKeyHash) {
            return true;
           }
        }
        return false;
    }

    constructor(uint64 durationSeconds) {
        require(durationSeconds > 0, "Duration must be > 0");
        defaultDuration = durationSeconds;
        dataNonce = 0;
    }


    function addToBlacklist(address account, bytes calldata crPublicKey, bytes calldata signature) external {
        require(account != address(0), "Zero address");
        require(verifySignature(crPublicKey, abi.encodePacked(account, dataNonce), signature), "Invalid signature");
        dataNonce += 1;
        Entry storage entry = entries[account];
        require(!entry.permanent, "Already permanent");

        entry.addedAt = uint64(block.timestamp);
        entry.permanent = false;
        entry.exists = true;

        emit Blacklisted(account, block.timestamp + defaultDuration);
    }

    function isBlacklisted(
        address account
    )
        public
        view
        returns (bool)
    {
        Entry memory entry = entries[account];
        if (!entry.exists) {
            return false;
        }

        if (entry.permanent) {
            return true;
        }

        uint256 expiresAt = uint256(entry.addedAt) + defaultDuration;
        return block.timestamp < expiresAt;
    }

    function getEntry(
        address account
    )
        external
        view
        returns (
            bool exists,
            bool permanent,
            uint256 addedAt,
            uint256 expiresAt,
            bool active
        )
    {
        Entry memory entry = entries[account];
        exists = entry.exists;
        permanent = entry.permanent;
        addedAt = entry.addedAt;
        if (entry.exists && !entry.permanent) {
            expiresAt = uint256(entry.addedAt) + defaultDuration;
            active = block.timestamp < expiresAt;
        } else if (entry.permanent) {
            active = true;
            expiresAt = type(uint256).max;
        }
    }

    function proposePermanent(address account, bytes calldata crPublicKey, bytes calldata signature) external returns (uint256) {
        Entry storage entry = entries[account];
        require(entry.exists, "Not blacklisted");
        require(!entry.permanent, "Already permanent");

        uint256 proposalId = _createProposal(Action.MakePermanent, account, crPublicKey, signature);
        _tryExecute(proposalId);
        return proposalId;
    }

    function proposeRemoval(address account, bytes calldata crPublicKey, bytes calldata signature) external returns (uint256) {
        Entry storage entry = entries[account];
        require(entry.exists, "Not blacklisted");

        uint256 proposalId = _createProposal(Action.Remove, account, crPublicKey, signature);
        _tryExecute(proposalId);
        return proposalId;
    }

    function approve(uint256 proposalId, bytes calldata crPublicKey, bytes calldata signature) external {
        _approve(proposalId, crPublicKey, signature);
        _tryExecute(proposalId);
    }

    function _createProposal(
        Action action,
        address target,
        bytes calldata crPublicKey,
        bytes calldata crSignature
    ) internal returns (uint256) {
        proposalCount += 1;
        uint256 proposalId = proposalCount;

        Proposal storage p = proposals[proposalId];
        p.action = action;
        p.target = target;
        p.approvals = 0;
        p.executed = false;

        emit ProposalCreated(proposalId, action, target, crPublicKey);
        _approve(proposalId, crPublicKey, crSignature);
        return proposalId;
    }

    function _approve(uint256 proposalId, bytes calldata crPublicKey, bytes calldata signature) internal {
        Proposal storage proposal = proposals[proposalId];
        require(proposal.target != address(0), "Proposal not found");
        require(!proposal.executed, "Already executed");
        require(!proposalVotes[proposalId][crPublicKey], "Already approved");
        require(verifySignature(crPublicKey, abi.encodePacked(proposalId), signature), "Invalid signature");
        proposalVotes[proposalId][crPublicKey] = true;
        proposal.approvals += 1;

        emit ProposalApproved(proposalId, crPublicKey, proposal.approvals);
    }

    function _tryExecute(uint256 proposalId) internal {
        Proposal storage proposal = proposals[proposalId];
        if (proposal.executed) {
            return;
        }

        if (proposal.approvals < THRESHOLD) {
            return;
        }

        proposal.executed = true;

        if (proposal.action == Action.Remove) {
            delete entries[proposal.target];
            emit BlacklistRemoved(proposal.target);
        } else if (proposal.action == Action.MakePermanent) {
            Entry storage entry = entries[proposal.target];
            entry.permanent = true;
            entry.exists = true;
            emit BlacklistPermanent(proposal.target);
        }
    }
}

