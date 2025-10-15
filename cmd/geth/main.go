// Copyright 2014 The Elastos ELA Side Chain ESC Authors
// This file is part of Elastos ELA Side Chain ESC.
//
// Elastos ELA Side Chain ESC is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Elastos ELA Side Chain ESC is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Elastos ELA Side Chain ESC. If not, see <http://www.gnu.org/licenses/>.

// geth is a command-line client for Elastos ELA Side Chain.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/elastos/Elastos.ELA.SideChain.ESC/core/events"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/eth"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/spv"
	"golang.org/x/crypto/ripemd160"

	"github.com/elastos/Elastos.ELA.SideChain.ESC/accounts"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/cmd/utils"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/common"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/console/prompt"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/eth/downloader"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/ethclient"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/internal/debug"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/internal/flags"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/log"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/node"

	// Force-load the tracer engines to trigger registration
	_ "github.com/elastos/Elastos.ELA.SideChain.ESC/eth/tracers/js"
	_ "github.com/elastos/Elastos.ELA.SideChain.ESC/eth/tracers/live"
	_ "github.com/elastos/Elastos.ELA.SideChain.ESC/eth/tracers/native"

	elacom "github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/core/contract"

	"github.com/urfave/cli/v2"
	"go.uber.org/automaxprocs/maxprocs"
)

const (
	clientIdentifier = "esc" // Client identifier to advertise over the network
)

var (
	// flags that configure the node
	nodeFlags = slices.Concat([]cli.Flag{
		utils.IdentityFlag,
		utils.UnlockedAccountFlag,
		utils.PasswordFileFlag,
		utils.BootnodesFlag,
		utils.MinFreeDiskSpaceFlag,
		utils.KeyStoreDirFlag,
		utils.ExternalSignerFlag,
		utils.NoUSBFlag, // deprecated
		utils.USBFlag,
		utils.SmartCardDaemonPathFlag,
		utils.OverrideOsaka,
		utils.OverrideBPO1,
		utils.OverrideBPO2,
		utils.OverrideVerkle,
		utils.EnablePersonal, // deprecated
		utils.TxPoolLocalsFlag,
		utils.TxPoolNoLocalsFlag,
		utils.TxPoolJournalFlag,
		utils.TxPoolRejournalFlag,
		utils.TxPoolPriceLimitFlag,
		utils.TxPoolPriceBumpFlag,
		utils.TxPoolAccountSlotsFlag,
		utils.TxPoolGlobalSlotsFlag,
		utils.TxPoolAccountQueueFlag,
		utils.TxPoolGlobalQueueFlag,
		utils.TxPoolLifetimeFlag,
		utils.BlobPoolDataDirFlag,
		utils.BlobPoolDataCapFlag,
		utils.BlobPoolPriceBumpFlag,
		utils.SyncModeFlag,
		utils.SyncTargetFlag,
		utils.ExitWhenSyncedFlag,
		utils.GCModeFlag,
		utils.SnapshotFlag,
		utils.TxLookupLimitFlag, // deprecated
		utils.TransactionHistoryFlag,
		utils.ChainHistoryFlag,
		utils.LogHistoryFlag,
		utils.LogNoHistoryFlag,
		utils.LogExportCheckpointsFlag,
		utils.StateHistoryFlag,
		utils.LightKDFFlag,
		utils.EthRequiredBlocksFlag,
		utils.LegacyWhitelistFlag, // deprecated
		utils.CacheFlag,
		utils.CacheDatabaseFlag,
		utils.CacheTrieFlag,
		utils.CacheTrieJournalFlag,   // deprecated
		utils.CacheTrieRejournalFlag, // deprecated
		utils.CacheGCFlag,
		utils.CacheSnapshotFlag,
		utils.CacheNoPrefetchFlag,
		utils.CachePreimagesFlag,
		utils.CacheLogSizeFlag,
		utils.FDLimitFlag,
		utils.CryptoKZGFlag,
		utils.ListenPortFlag,
		utils.DiscoveryPortFlag,
		utils.MaxPeersFlag,
		utils.MaxPendingPeersFlag,
		utils.MiningEnabledFlag, // deprecated
		utils.MinerGasLimitFlag,
		utils.MinerGasPriceFlag,
		utils.MinerEtherbaseFlag, // deprecated
		utils.MinerExtraDataFlag,
		utils.MinerRecommitIntervalFlag,
		utils.MinerPendingFeeRecipientFlag,
		utils.MinerNewPayloadTimeoutFlag, // deprecated
		utils.NATFlag,
		utils.NoDiscoverFlag,
		utils.DiscoveryV4Flag,
		utils.DiscoveryV5Flag,
		utils.LegacyDiscoveryV5Flag, // deprecated
		utils.NetrestrictFlag,
		utils.NodeKeyFileFlag,
		utils.NodeKeyHexFlag,
		utils.DNSDiscoveryFlag,
		utils.DeveloperFlag,
		utils.DeveloperGasLimitFlag,
		utils.DeveloperPeriodFlag,
		utils.VMEnableDebugFlag,
		utils.VMTraceFlag,
		utils.VMTraceJsonConfigFlag,
		utils.VMWitnessStatsFlag,
		utils.VMStatelessSelfValidationFlag,
		utils.NetworkIdFlag,
		utils.EthStatsURLFlag,
		utils.GpoBlocksFlag,
		utils.GpoPercentileFlag,
		utils.GpoMaxGasPriceFlag,
		utils.GpoIgnoreGasPriceFlag,
		configFileFlag,
		utils.LogDebugFlag,
		utils.LogBacktraceAtFlag,
		utils.BeaconApiFlag,
		utils.BeaconApiHeaderFlag,
		utils.BeaconThresholdFlag,
		utils.BeaconNoFilterFlag,
		utils.BeaconConfigFlag,
		utils.BeaconGenesisRootFlag,
		utils.BeaconGenesisTimeFlag,
		utils.BeaconCheckpointFlag,
		utils.BeaconCheckpointFileFlag,
	}, utils.NetworkFlags, utils.DatabaseFlags)

	rpcFlags = []cli.Flag{
		utils.HTTPEnabledFlag,
		utils.HTTPListenAddrFlag,
		utils.HTTPPortFlag,
		utils.HTTPCORSDomainFlag,
		utils.AuthListenFlag,
		utils.AuthPortFlag,
		utils.AuthVirtualHostsFlag,
		utils.JWTSecretFlag,
		utils.HTTPVirtualHostsFlag,
		utils.GraphQLEnabledFlag,
		utils.GraphQLCORSDomainFlag,
		utils.GraphQLVirtualHostsFlag,
		utils.HTTPApiFlag,
		utils.HTTPPathPrefixFlag,
		utils.WSEnabledFlag,
		utils.WSListenAddrFlag,
		utils.WSPortFlag,
		utils.WSApiFlag,
		utils.WSAllowedOriginsFlag,
		utils.WSPathPrefixFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
		utils.InsecureUnlockAllowedFlag,
		utils.RPCGlobalGasCapFlag,
		utils.RPCGlobalEVMTimeoutFlag,
		utils.RPCGlobalTxFeeCapFlag,
		utils.RPCGlobalLogQueryLimit,
		utils.AllowUnprotectedTxs,
		utils.BatchRequestLimit,
		utils.BatchResponseMaxSize,
	}

	metricsFlags = []cli.Flag{
		utils.MetricsEnabledFlag,
		utils.MetricsEnabledExpensiveFlag,
		utils.MetricsHTTPFlag,
		utils.MetricsPortFlag,
		utils.MetricsEnableInfluxDBFlag,
		utils.MetricsInfluxDBEndpointFlag,
		utils.MetricsInfluxDBDatabaseFlag,
		utils.MetricsInfluxDBUsernameFlag,
		utils.MetricsInfluxDBPasswordFlag,
		utils.MetricsInfluxDBTagsFlag,
		utils.MetricsEnableInfluxDBV2Flag,
		utils.MetricsInfluxDBTokenFlag,
		utils.MetricsInfluxDBBucketFlag,
		utils.MetricsInfluxDBOrganizationFlag,
		utils.StateSizeTrackingFlag,
	}
)

var app = flags.NewApp("the Elastos ELA Side Chain ESC command line interface")

func init() {
	// Initialize the CLI app and start Elastos ELA Side Chain
	app.Action = geth
	app.Commands = []*cli.Command{
		// See chaincmd.go:
		initCommand,
		importCommand,
		exportCommand,
		importHistoryCommand,
		exportHistoryCommand,
		importPreimagesCommand,
		removedbCommand,
		dumpCommand,
		dumpGenesisCommand,
		pruneHistoryCommand,
		downloadEraCommand,
		// See accountcmd.go:
		accountCommand,
		walletCommand,
		// See consolecmd.go:
		consoleCommand,
		attachCommand,
		javascriptCommand,
		// See misccmd.go:
		versionCommand,
		versionCheckCommand,
		licenseCommand,
		// See config.go
		dumpConfigCommand,
		// see dbcmd.go
		dbCommand,
		// See cmd/utils/flags_legacy.go
		utils.ShowDeprecated,
		// See snapshot.go
		snapshotCommand,
		// See verkle.go
		verkleCommand,
	}
	if logTestCommand != nil {
		app.Commands = append(app.Commands, logTestCommand)
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = slices.Concat(
		nodeFlags,
		rpcFlags,
		consoleFlags,
		debug.Flags,
		metricsFlags,
	)
	flags.AutoEnvVars(app.Flags, "GETH")

	app.Before = func(ctx *cli.Context) error {
		maxprocs.Set() // Automatically set GOMAXPROCS to match Linux container CPU quota.
		flags.MigrateGlobalFlags(ctx)
		if err := debug.Setup(ctx); err != nil {
			return err
		}
		flags.CheckEnvVars(ctx, app.Flags, "GETH")
		return nil
	}
	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		prompt.Stdin.Close() // Resets terminal mode.
		return nil
	}
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// prepare manipulates memory cache allowance and setups metric system.
// This function should be called before launching devp2p stack.
func prepare(ctx *cli.Context) {
	// If we're running a known preset, log it for convenience.
	switch {
	case ctx.IsSet(utils.SepoliaFlag.Name):
		log.Info("Starting Elastos ELA Side Chain on Sepolia testnet...")

	case ctx.IsSet(utils.HoleskyFlag.Name):
		log.Info("Starting Elastos ELA Side Chain on Holesky testnet...")

	case ctx.IsSet(utils.HoodiFlag.Name):
		log.Info("Starting Elastos ELA Side Chain on Hoodi testnet...")

	case !ctx.IsSet(utils.NetworkIdFlag.Name):
		log.Info("Starting Elastos ELA Side Chain on mainnet...")
	}
	// If we're a full node on mainnet without --cache specified, bump default cache allowance
	if !ctx.IsSet(utils.CacheFlag.Name) && !ctx.IsSet(utils.NetworkIdFlag.Name) {
		// Make sure we're not on any supported preconfigured testnet either
		if !ctx.IsSet(utils.HoleskyFlag.Name) &&
			!ctx.IsSet(utils.SepoliaFlag.Name) &&
			!ctx.IsSet(utils.HoodiFlag.Name) &&
			!ctx.IsSet(utils.DeveloperFlag.Name) {
			// Nope, we're really on mainnet. Bump that cache up!
			log.Info("Bumping default cache on mainnet", "provided", ctx.Int(utils.CacheFlag.Name), "updated", 4096)
			ctx.Set(utils.CacheFlag.Name, strconv.Itoa(4096))
		}
	}
}

// geth is the main entry point into the system if no special subcommand is run.
// It creates a default node based on the command line arguments and runs it in
// blocking mode, waiting for it to be shut down.
func geth(ctx *cli.Context) error {
	if args := ctx.Args().Slice(); len(args) > 0 {
		return fmt.Errorf("invalid command: %q", args[0])
	}

	prepare(ctx)
	stack, eth := makeFullNode(ctx)
	defer stack.Close()

	startNode(ctx, stack, eth, false)
	stack.Wait()
	return nil
}

// startNode boots up the system node and all registered protocols, after which
// it starts the RPC/IPC interfaces and the miner.
func startNode(ctx *cli.Context, stack *node.Node, eth *eth.Ethereum, isConsole bool) {
	// Start up the node itself
	utils.StartNode(ctx, stack, isConsole)

	if ctx.IsSet(utils.UnlockedAccountFlag.Name) {
		log.Warn(`The "unlock" flag has been deprecated and has no effect`)
	}

	// Register wallet event handlers to open and auto-derive wallets
	events := make(chan accounts.WalletEvent, 16)
	stack.AccountManager().Subscribe(events)

	// Create a client to interact with local geth node.
	rpcClient := stack.Attach()
	ethClient := ethclient.NewClient(rpcClient)

	go func() {
		// Open any wallets already attached
		for _, wallet := range stack.AccountManager().Wallets() {
			if err := wallet.Open(""); err != nil {
				log.Warn("Failed to open wallet", "url", wallet.URL(), "err", err)
			}
		}
		// Listen for wallet event till termination
		for event := range events {
			switch event.Kind {
			case accounts.WalletArrived:
				if err := event.Wallet.Open(""); err != nil {
					log.Warn("New wallet appeared, failed to open", "url", event.Wallet.URL(), "err", err)
				}
			case accounts.WalletOpened:
				status, _ := event.Wallet.Status()
				log.Info("New wallet appeared", "url", event.Wallet.URL(), "status", status)

				var derivationPaths []accounts.DerivationPath
				if event.Wallet.URL().Scheme == "ledger" {
					derivationPaths = append(derivationPaths, accounts.LegacyLedgerBaseDerivationPath)
				}
				derivationPaths = append(derivationPaths, accounts.DefaultBaseDerivationPath)

				event.Wallet.SelfDerive(derivationPaths, ethClient)

			case accounts.WalletDropped:
				log.Info("Old wallet dropped", "url", event.Wallet.URL())
				event.Wallet.Close()
			}
		}
	}()

	// Spawn a standalone goroutine for status synchronization monitoring,
	// close the node when synchronization is complete if user required.
	if ctx.Bool(utils.ExitWhenSyncedFlag.Name) {
		go func() {
			sub := stack.EventMux().Subscribe(downloader.DoneEvent{})
			defer sub.Unsubscribe()
			for {
				event := <-sub.Chan()
				if event == nil {
					continue
				}
				done, ok := event.Data.(downloader.DoneEvent)
				if !ok {
					continue
				}
				if timestamp := time.Unix(int64(done.Latest.Time), 0); time.Since(timestamp) < 10*time.Minute {
					log.Info("Synchronisation completed", "latestnum", done.Latest.Number, "latesthash", done.Latest.Hash(),
						"age", common.PrettyAge(timestamp))
					stack.Close()
				}
			}
		}()
	}

	startSpv(ctx, ethClient, stack, eth)
}

func startSpv(ctx *cli.Context, client *ethclient.Client, stack *node.Node, eth *eth.Ethereum) {
	var SpvDataDir string
	switch {
	case ctx.IsSet(utils.DataDirFlag.Name):
		SpvDataDir = ctx.String(utils.DataDirFlag.Name)
	case ctx.Bool(utils.SepoliaFlag.Name):
		SpvDataDir = filepath.Join(node.DefaultDataDir(), "testnet")
	case ctx.Bool(utils.HoleskyFlag.Name):
		SpvDataDir = filepath.Join(node.DefaultDataDir(), "regnet")
	case ctx.Bool(utils.MainnetFlag.Name):
		SpvDataDir = node.DefaultDataDir()
	default:
		SpvDataDir = ""
	}

	var spvCfg = &spv.Config{
		DataDir:   SpvDataDir,
		ActiveNet: "",
	}
	// prepare the SPV service config parameters
	switch {
	case ctx.Bool(utils.SepoliaFlag.Name):
		spvCfg.ActiveNet = "t"
	case ctx.Bool(utils.HoleskyFlag.Name):
		spvCfg.ActiveNet = "r"
	}

	// prepare to start the SPV module
	// if --spvmoniaddr commandline parameter is present, use the parameter value
	// as the ELA mainchain address for the SPV module to monitor on
	// if no --spvmoniaddr commandline parameter is provided, use the sidechain genesis block hash
	// to generate the corresponding ELA mainchain address for the SPV module to monitor on
	var dynamicArbiterHeight uint64
	var pledgedBillContract string
	if ctx.String(utils.SpvMonitoringAddrFlag.Name) != "" {
		// --spvmoniaddr parameter is provided, set the SPV monitor address accordingly
		log.Info("SPV Start Monitoring... ", "SpvMonitoringAddr", ctx.String(utils.SpvMonitoringAddrFlag.Name))
		spvCfg.GenesisAddress = ctx.String(utils.SpvMonitoringAddrFlag.Name)
	} else {
		// --spvmoniaddr parameter is not provided
		// get the Ethereum node service to get the genesis block hash
		ghash := eth.BlockChain().Genesis().Hash()

		dynamicArbiterHeight = eth.BlockChain().Config().DynamicArbiterHeight
		pledgedBillContract = eth.BlockChain().Config().PledgeBillContract

		// calculate ELA mainchain address from the genesis block hash and set the SPV monitor address accordingly
		genesisU256, err := elacom.Uint256FromBytes(ghash.Bytes())
		if err != nil {
			utils.Fatalf("Blockchain not running: %v", err)
		}
		spvCfg.GenesisHash = *genesisU256
		log.Info(fmt.Sprintf("Genesis block hash: %v uint256 fromat:%v", ghash.String(), genesisU256.String()))
		if gaddr, err := calculateGenesisAddress(ghash.String()); err != nil {
			utils.Fatalf("Cannot calculate: %v", err)
		} else {
			log.Info(fmt.Sprintf("SPV Start Monitoring... : %v", gaddr))
			spvCfg.GenesisAddress = gaddr
		}
	}
	spv.GetDefaultSingerAddr = func() common.Address {
		var addr common.Address
		if wallets := stack.AccountManager().Wallets(); len(wallets) > 0 {
			if accounts := wallets[0].Accounts(); len(accounts) > 0 {
				addr = accounts[0].Address
			}
		}

		return addr
	}
	spv.SpvDbInit(SpvDataDir, pledgedBillContract, spv.GetDefaultSingerAddr(), client)
	if spvService, err := spv.NewService(spvCfg, stack.EventMux(), dynamicArbiterHeight); err != nil {
		utils.Fatalf("SPV service init error: %v", err)
	} else {
		MinedBlockSub := stack.EventMux().Subscribe(events.MinedBlockEvent{})
		OnDutySub := stack.EventMux().Subscribe(events.OnDutyEvent{})
		smallCroTxSub := stack.EventMux().Subscribe(events.CmallCrossTx{})
		go spv.MinedBroadcastLoop(MinedBlockSub, OnDutySub, smallCroTxSub)
		spvService.Start()
		stack.EventMux().Post(events.InitCurrentProducers{})
		spv.InitNextTurnDposInfo()
	}
}

// calculate the ELA mainchain address from the sidechain (ie. this chain)
// genesis block hash for corresponding crosschain transactions
// refer to https://github.com/elastos/Elastos.ELA.Client/blob/dev/cli/wallet/wallet.go
// for the original ELA-CLI implementation
func calculateGenesisAddress(genesisBlockHash string) (string, error) {
	// unlike Ethereum, the ELA hash values do not contain 0x prefix
	if strings.HasPrefix(genesisBlockHash, "0x") {
		genesisBlockHash = genesisBlockHash[2:]
	}
	genesisBlockBytes, err := hex.DecodeString(genesisBlockHash)
	if err != nil {
		return "", errors.New("genesis block hash to bytes failed")
	}
	reversedGenesisBlockBytes := elacom.BytesReverse(genesisBlockBytes)
	reversedGenesisBlockStr := elacom.BytesToHexString(reversedGenesisBlockBytes)

	log.Info(fmt.Sprintf("genesis program hash: %v", reversedGenesisBlockStr))

	buf := new(bytes.Buffer)
	buf.WriteByte(byte(len(reversedGenesisBlockBytes)))
	buf.Write(reversedGenesisBlockBytes)
	buf.WriteByte(byte(elacom.CROSSCHAIN))

	sum168 := func(prefix byte, code []byte) []byte {
		hash := sha256.Sum256(code)
		md160 := ripemd160.New()
		md160.Write(hash[:])
		return md160.Sum([]byte{prefix})
	}
	genesisProgramHash, err := elacom.Uint168FromBytes(sum168(byte(contract.PrefixCrossChain), buf.Bytes()))
	if err != nil {
		return "", errors.New("genesis block bytes to program hash failed")
	}

	genesisAddress, err := genesisProgramHash.ToAddress()
	if err != nil {
		return "", errors.New("genesis block hash to genesis address failed")
	}
	log.Info(fmt.Sprintf("genesis address: %v ", genesisAddress))

	return genesisAddress, nil
}
