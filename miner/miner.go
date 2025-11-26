// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/elastos/Elastos.ELA.SideChain.ESC/common"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/consensus"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/core"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/core/state"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/core/txpool"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/core/types"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/eth/downloader"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/event"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/log"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/miner/minerconfig"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/params"
)

// Backend wraps all methods required for mining. Only full node is capable
// to offer all the functions here.
type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *txpool.TxPool
}

// Miner is the main object which takes care of submitting new work to consensus
// engine and gathering the sealing result.
type Miner struct {
	mux     *event.TypeMux
	eth     Backend
	exitCh  chan struct{}
	startCh chan struct{}
	stopCh  chan struct{}
	worker  *worker

	wg sync.WaitGroup
}

// New creates a new miner with provided config.
func New(eth Backend, config *minerconfig.Config, mux *event.TypeMux, engine consensus.Engine) *Miner {
	miner := &Miner{
		mux:     mux,
		eth:     eth,
		exitCh:  make(chan struct{}),
		startCh: make(chan struct{}),
		stopCh:  make(chan struct{}),
		worker:  newWorker(config, engine, eth, mux),
	}

	miner.wg.Add(1)
	go miner.update()
	return miner
}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (miner *Miner) update() {
	defer miner.wg.Done()

	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer func() {
		if !events.Closed() {
			events.Unsubscribe()
		}
	}()

	shouldStart := false
	canStart := true
	dlEventCh := events.Chan()
	for {
		select {
		case ev := <-dlEventCh:
			if ev == nil {
				// Unsubscription done, stop listening
				dlEventCh = nil
				continue
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				wasMining := miner.Mining()
				miner.worker.stop()
				canStart = false
				if wasMining {
					// Resume mining after sync was finished
					shouldStart = true
					log.Info("Mining aborted due to sync")
				}
				miner.worker.syncing.Store(true)

			case downloader.FailedEvent:
				canStart = true
				if shouldStart {
					miner.worker.start()
				}
				miner.worker.syncing.Store(false)

			case downloader.DoneEvent:
				canStart = true
				if shouldStart {
					miner.worker.start()
				}
				miner.worker.syncing.Store(false)

				// Stop reacting to downloader events
				events.Unsubscribe()
			}
		case <-miner.startCh:
			if canStart {
				miner.worker.start()
			}
			shouldStart = true
		case <-miner.stopCh:
			shouldStart = false
			miner.worker.stop()
		case <-miner.exitCh:
			miner.worker.close()
			return
		}
	}
}

func (miner *Miner) Start(coinbase common.Address) {
	miner.SetEtherbase(coinbase)
	miner.startCh <- struct{}{}
}

func (miner *Miner) Stop() {
	miner.stopCh <- struct{}{}
}

func (miner *Miner) Close() {
	close(miner.exitCh)
	miner.wg.Wait()
}

func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}

// Pending returns the latest block and associated receipts, logs
// and statedb. The returned values can be nil in case the pending block is
// not initialized.
func (miner *Miner) Pending() (*types.Block, types.Receipts, *state.StateDB) {
	block := miner.worker.chain.CurrentBlock()
	if block == nil {
		return nil, nil, nil
	}
	stateDb, err := miner.worker.chain.StateAt(block.Root)
	if err != nil {
		return nil, nil, nil
	}
	return miner.worker.chain.GetBlockByHash(block.Hash()), miner.worker.chain.GetReceiptsByHash(block.Hash()), stateDb
}

// SetExtra sets the content used to initialize the block extra field.
func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
}

func (miner *Miner) SetGasTip(tip *big.Int) error {
	miner.worker.setGasTip(tip)
	return nil
}

// SetRecommitInterval sets the interval for sealing work resubmitting.
func (miner *Miner) SetRecommitInterval(interval time.Duration) {
	miner.worker.setRecommitInterval(interval)
}

func (miner *Miner) SetEtherbase(addr common.Address) {
	miner.worker.setEtherbase(addr)
}

// SetPrioAddresses sets a list of addresses to prioritize for transaction inclusion.
func (miner *Miner) SetPrioAddresses(prio []common.Address) {
	miner.worker.setPrioAddresses(prio)
}

// SetGasCeil sets the gaslimit to strive for when mining blocks post 1559.
// For pre-1559 blocks, it sets the ceiling.
func (miner *Miner) SetGasCeil(ceil uint64) {
	miner.worker.setGasCeil(ceil)
}

// BuildPayload builds the payload according to the provided parameters.
func (miner *Miner) BuildPayload(args *BuildPayloadArgs, witness bool) (*Payload, error) {
	return miner.worker.buildPayload(args, witness)
}

func (miner *Miner) GasCeil() uint64 {
	return miner.worker.getGasCeil()
}

func (miner *Miner) SetEngine(engine consensus.Engine) {
	miner.worker.SetEngine(engine)
}
