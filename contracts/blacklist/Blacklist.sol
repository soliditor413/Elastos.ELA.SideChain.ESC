// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;
import {console} from "hardhat/console.sol";
/**
 * @title BlacklistV3 - 黑名单合约
 * @notice
 *   - 未锁定/临时锁定地址：可以投 add（累积到永久阈值）
 *   - 永久锁定地址：不能投 add
 *   - 已锁定地址（临时或永久）：可以投 remove
 *   - 不支持切换投票（add/remove 是独立的投票池）
 *   - 配置修改需要 CR 委员投票
 */
contract Blacklist {
    // ============ 常量 ============
    uint256 private constant MAX_UINT256 = type(uint256).max;
    uint64 public constant MAX_THRESHOLD = 12;  // CR 委员会最大人数
    address private constant CR_MEMBERS_PRECOMPILE = address(1000);
    address private constant SIGNATURE_PRECOMPILE = address(1009);

    // ============ 配置参数（CR 投票修改）============
    uint64 public tempThreshold;     // 临时锁定票数
    uint64 public permThreshold;     // 永久锁定/解锁票数
    uint64 public freezeDuration;    // 临时锁定时长

    // ============ 黑名单数据 ============
    struct Entry {
        uint256 expiresAt;    // 0=未锁定, MAX=永久, 其他=截止时间
        uint64 epoch;         // 投票轮次
        uint64 addCount;      // add 票数
        uint64 removeCount;   // remove 票数
    }

    mapping(address => Entry) private entries;

    // 投票记录：分开存储 add 和 remove
    mapping(address => mapping(uint64 => mapping(bytes32 => bool))) private addVotes;
    mapping(address => mapping(uint64 => mapping(bytes32 => bool))) private removeVotes;

    // Nonce：防止签名重放
    mapping(address => mapping(bytes32 => uint64)) public nonces;

    // ============ 配置变更提案 ============
    struct ConfigProposal {
        uint64 tempThreshold;
        uint64 permThreshold;
        uint64 freezeDuration;
        uint64 voteCount;
        bool executed;
    }

    uint64 public configEpoch;
    ConfigProposal public pendingConfig;
    mapping(uint64 => mapping(bytes32 => bool)) private configVotes;

    // ============ 事件 ============
    event AddVoted(address indexed target, bytes32 indexed voter, uint64 epoch, uint64 addCount);
    event RemoveVoted(address indexed target, bytes32 indexed voter, uint64 epoch, uint64 removeCount);
    event TempLocked(address indexed target, uint256 expiresAt);
    event PermLocked(address indexed target);
    event Unlocked(address indexed target, uint64 newEpoch);
    event ConfigProposed(uint64 indexed epoch, uint64 temp, uint64 perm, uint64 duration, bytes32 indexed proposer);
    event ConfigVoted(uint64 indexed epoch, bytes32 indexed voter, uint64 voteCount);
    event ConfigApplied(uint64 indexed epoch, uint64 temp, uint64 perm, uint64 duration);

    // ============ 错误 ============
    error ZeroAddress();
    error InvalidThresholds();
    error ThresholdTooHigh();      // permThreshold > MAX_THRESHOLD
    error InvalidDuration();
    error NotLocked();
    error AlreadyPermanent();      // 永久锁定后不能再投 add
    error AlreadyVoted();
    error InvalidSignature();
    error NotCRMember();
    error NoActiveProposal();
    error ProposalAlreadyExecuted();

    // ============ 构造函数 ============
    constructor(uint64 _temp, uint64 _perm, uint64 _duration) {
        if (_temp == 0 || _temp > _perm) revert InvalidThresholds();
        if (_perm > MAX_THRESHOLD) revert ThresholdTooHigh();
        if (_duration == 0) revert InvalidDuration();
        tempThreshold = _temp;
        permThreshold = _perm;
        freezeDuration = _duration;
    }

    // ============ 黑名单核心函数 ============

    /**
     * @notice 投票锁定地址（未锁定或临时锁定的地址可继续累积票数）
     * @param target 目标地址
     * @param crPublicKey CR 委员 33 字节压缩公钥
     * @param signature 65 字节签名
     */
    function voteAdd(
        address target,
        bytes calldata crPublicKey,
        bytes calldata signature
    ) external {
        if (target == address(0)) revert ZeroAddress();

        _checkAndResetIfExpired(target);

        Entry storage e = entries[target];

        // 永久锁定的地址不能再投 add（临时锁定可以继续投票直到达到永久阈值）
        if (e.expiresAt == MAX_UINT256) revert AlreadyPermanent();

        bytes32 keyHash = keccak256(crPublicKey);

        // 检查是否已投票
        if (addVotes[target][e.epoch][keyHash]) revert AlreadyVoted();

        // 验证签名
        uint64 nonce = nonces[target][keyHash];
        _verifySignature(crPublicKey, abi.encodePacked(
            address(this), target, e.epoch, nonce, "add"
        ), signature);
        nonces[target][keyHash] = nonce + 1;

        // 记录投票
        addVotes[target][e.epoch][keyHash] = true;
        e.addCount++;

        emit AddVoted(target, keyHash, e.epoch, e.addCount);

        // 检查阈值并更新状态
        if (e.addCount >= permThreshold) {
            // 达到永久阈值：升级为永久锁定
            e.expiresAt = MAX_UINT256;
            emit PermLocked(target);
        } else if (e.addCount >= tempThreshold && e.expiresAt == 0) {
            // 首次达到临时阈值：开始临时锁定（已是临时锁定的不重复设置）
            e.expiresAt = block.timestamp + freezeDuration;
            emit TempLocked(target, e.expiresAt);
        }
    }

    /**
     * @notice 投票解锁地址（仅限已锁定的地址）
     * @param target 目标地址
     * @param crPublicKey CR 委员 33 字节压缩公钥
     * @param signature 65 字节签名
     */
    function voteRemove(
        address target,
        bytes calldata crPublicKey,
        bytes calldata signature
    ) external {
        if (target == address(0)) revert ZeroAddress();
        _checkAndResetIfExpired(target);

        Entry storage e = entries[target];

        // 只能对已锁定的地址投 remove
        if (e.expiresAt == 0) revert NotLocked();

        bytes32 keyHash = keccak256(crPublicKey);

        // 检查是否已投票 remove
        if (removeVotes[target][e.epoch][keyHash]) revert AlreadyVoted();

        // 验证签名
        uint64 nonce = nonces[target][keyHash];
        _verifySignature(crPublicKey, abi.encodePacked(
            address(this), target, e.epoch, nonce, "remove"
        ), signature);
        nonces[target][keyHash] = nonce + 1;

        // 记录投票
        removeVotes[target][e.epoch][keyHash] = true;
        e.removeCount++;

        emit RemoveVoted(target, keyHash, e.epoch, e.removeCount);

        // 检查是否达到解锁阈值
        if (e.removeCount >= permThreshold) {
            _resetEntry(target);
        }
    }

    // ============ 配置管理函数 ============

    /**
     * @notice 发起配置变更提案
     * @param _temp 新的临时锁定阈值
     * @param _perm 新的永久锁定/解锁阈值
     * @param _duration 新的临时锁定时长
     * @param crPublicKey CR 委员公钥
     * @param signature 签名
     */
    function proposeConfig(
        uint64 _temp,
        uint64 _perm,
        uint64 _duration,
        bytes calldata crPublicKey,
        bytes calldata signature
    ) external {
        if (_temp == 0 || _temp > _perm) revert InvalidThresholds();
        if (_perm > MAX_THRESHOLD) revert ThresholdTooHigh();
        if (_duration == 0) revert InvalidDuration();
        bytes32 keyHash = keccak256(crPublicKey);

        // 验证签名（包含新的 configEpoch + 1）
        _verifySignature(crPublicKey, abi.encodePacked(
            address(this), "config", configEpoch + 1, _temp, _perm, _duration
        ), signature);

        // 新提案覆盖旧提案
        configEpoch++;
        pendingConfig = ConfigProposal({
            tempThreshold: _temp,
            permThreshold: _perm,
            freezeDuration: _duration,
            voteCount: 1,
            executed: false
        });

        configVotes[configEpoch][keyHash] = true;

        emit ConfigProposed(configEpoch, _temp, _perm, _duration, keyHash);
        emit ConfigVoted(configEpoch, keyHash, 1);

        _tryApplyConfig();
    }

    /**
     * @notice 投票支持当前配置提案
     * @param crPublicKey CR 委员公钥
     * @param signature 签名
     */
    function voteConfig(
        bytes calldata crPublicKey,
        bytes calldata signature
    ) external {
        if (pendingConfig.executed) revert ProposalAlreadyExecuted();
        if (configEpoch == 0) revert NoActiveProposal();

        bytes32 keyHash = keccak256(crPublicKey);
        if (configVotes[configEpoch][keyHash]) revert AlreadyVoted();

        // 验证签名（必须包含当前提案的具体参数）
        ConfigProposal storage p = pendingConfig;
        _verifySignature(crPublicKey, abi.encodePacked(
            address(this), "config", configEpoch, p.tempThreshold, p.permThreshold, p.freezeDuration
        ), signature);

        configVotes[configEpoch][keyHash] = true;
        p.voteCount++;

        emit ConfigVoted(configEpoch, keyHash, p.voteCount);

        _tryApplyConfig();
    }

    /**
     * @dev 检查并应用配置变更
     */
    function _tryApplyConfig() private {
        ConfigProposal storage p = pendingConfig;
        if (p.executed || p.voteCount < permThreshold) return;

        tempThreshold = p.tempThreshold;
        permThreshold = p.permThreshold;
        freezeDuration = p.freezeDuration;
        p.executed = true;

        emit ConfigApplied(configEpoch, p.tempThreshold, p.permThreshold, p.freezeDuration);
    }

    // ============ 查询函数 ============

    /**
     * @notice 检查地址是否在黑名单中
     */
    function isBlacklisted(address target) public view returns (bool) {
        Entry memory e = entries[target];
        if (e.expiresAt == 0) return false;
        if (e.expiresAt == MAX_UINT256) return true;
        return block.timestamp < e.expiresAt;
    }

    /**
     * @notice 获取地址的完整黑名单信息
     */
    function getEntry(address target) external view returns (
        bool locked,
        bool permanent,
        uint256 expiresAt,
        uint64 epoch,
        uint64 addCount,
        uint64 removeCount
    ) {
        Entry memory e = entries[target];
        locked = isBlacklisted(target);
        permanent = e.expiresAt == MAX_UINT256;
        expiresAt = e.expiresAt;
        epoch = e.epoch;
        addCount = e.addCount;
        removeCount = e.removeCount;
    }

    /**
     * @notice 获取委员对某地址的投票状态
     */
    function hasVoted(address target, bytes calldata crPublicKey) external view returns (
        bool votedAdd,
        bool votedRemove,
        uint64 nonce
    ) {
        Entry memory e = entries[target];
        bytes32 keyHash = keccak256(crPublicKey);
        votedAdd = addVotes[target][e.epoch][keyHash];
        votedRemove = removeVotes[target][e.epoch][keyHash];
        nonce = nonces[target][keyHash];
    }

    /**
     * @notice 获取当前配置提案状态
     */
    function getConfigProposal() external view returns (
        uint64 epoch,
        uint64 temp,
        uint64 perm,
        uint64 duration,
        uint64 voteCount,
        bool executed
    ) {
        ConfigProposal memory p = pendingConfig;
        epoch = configEpoch;
        temp = p.tempThreshold;
        perm = p.permThreshold;
        duration = p.freezeDuration;
        voteCount = p.voteCount;
        executed = p.executed;
    }

    /**
     * @notice 批量检查地址是否在黑名单
     */
    function batchIsBlacklisted(address[] calldata targets) external view returns (bool[] memory results) {
        results = new bool[](targets.length);
        for (uint256 i = 0; i < targets.length; i++) {
            results[i] = isBlacklisted(targets[i]);
        }
    }

    // ============ 内部函数 ============

    /**
     * @dev 检查临时锁定是否过期，过期则重置
     */
    function _checkAndResetIfExpired(address target) private {
        Entry storage e = entries[target];
        if (e.expiresAt > 0 && e.expiresAt < MAX_UINT256 && block.timestamp >= e.expiresAt) {
            _resetEntry(target);
        }
    }

    /**
     * @dev 重置条目状态
     */
    function _resetEntry(address target) private {
        Entry storage e = entries[target];
        uint64 newEpoch = e.epoch + 1;
        e.expiresAt = 0;
        e.epoch = newEpoch;
        e.addCount = 0;
        e.removeCount = 0;
        emit Unlocked(target, newEpoch);
    }

    /**
     * @dev 验证 CR 委员签名
     */
    function _verifySignature(
        bytes calldata pubKey,
        bytes memory message,
        bytes calldata sig
    ) private view {
        if (pubKey.length != 33) revert InvalidSignature();
        if (sig.length != 65) revert InvalidSignature();
        if (!_isCRMember(pubKey)) revert NotCRMember();

        bytes32 digest = sha256(message);
        (bool ok, bytes memory res) = SIGNATURE_PRECOMPILE.staticcall(
            abi.encodePacked(pubKey, digest, sig)
        );
        if (!ok || res.length == 0 || uint256(bytes32(res)) != 1) {
            revert InvalidSignature();
        }
    }

    /**
     * @dev 检查公钥是否是当前 CR 委员
     */
    function _isCRMember(bytes calldata pubKey) private view returns (bool) {
        (bool ok, bytes memory res) = CR_MEMBERS_PRECOMPILE.staticcall("");
        if (!ok || res.length == 0 || res.length % 32 != 0) return false;

        bytes32 hash = keccak256(pubKey);
        uint256 len = res.length / 32;
        for (uint256 i = 0; i < len; i++) {
            bytes32 slot;
            assembly {
                slot := mload(add(add(res, 0x20), mul(i, 0x20)))
            }
            if (slot == hash) return true;
        }
        return false;
    }
}
