// Copyright 2018 The go-ethereum Authors
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

package ethash

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

var (
	errEthashStopped      = errors.New("ethash stopped")
	errInvalidParentHash  = errors.New("invalid parent hash")
	errInvalidBlockNumber = errors.New("invalid block number")
)

// API exposes ethash related methods for the RPC interface.
type API struct {
	ethash *Ethash
}

// GetWork returns a work package for external miner.
//
// The work package consists of 3 strings:
//   result[0] - 32 bytes hex encoded current block header pow-hash
//   result[1] - 32 bytes hex encoded seed hash used for DAG
//   result[2] - 32 bytes hex encoded boundary condition ("target"), 2^256/difficulty
//   result[3] - hex encoded block number
func (api *API) GetWork() ([4]string, error) {
	if api.ethash.remote == nil {
		return [4]string{}, errors.New("not supported")
	}

	var (
		workCh = make(chan [4]string, 1)
		errc   = make(chan error, 1)
	)
	select {
	case api.ethash.remote.fetchWorkCh <- &sealWork{errc: errc, res: workCh}:
	case <-api.ethash.remote.exitCh:
		return [4]string{}, errEthashStopped
	}
	select {
	case work := <-workCh:
		return work, nil
	case err := <-errc:
		return [4]string{}, err
	}
}

// GetShardingWork returns a work package for external miner.
func (api *API) GetShardingWork(parentHash common.Hash, blockNumber uint64) ([4]string, error) {
	emptyRes := [4]string{}
	if api.ethash.remote == nil {
		return [4]string{}, errors.New("not supported")
	}

	var (
		workCh = make(chan [4]string, 1)
		errc   = make(chan error, 1)
	)
	select {
	case api.ethash.remote.fetchWorkCh <- &sealWork{errc: errc, res: workCh}:
	case <-api.ethash.remote.exitCh:
		return emptyRes, errEthashStopped
	}
	select {
	case work := <-workCh:
		curBlockHeader := api.ethash.remote.currentBlock.Header()
		if curBlockHeader != nil {
			log.Debug("Current Block Header Data", "time", curBlockHeader.Time, "block number", curBlockHeader.Number)
			// When producing block #1, validator does not know about hash of block #0
			// so do not check the parent hash and block number 1
			if blockNumber == 1 {
				return work, nil
			}
			if curBlockHeader.ParentHash != parentHash {
				log.Error("Mis-match in parentHash",
					"blockNumber", curBlockHeader.Number.Uint64(),
					"remoteParentHash", curBlockHeader.ParentHash, "receivedParentHash", parentHash)
				return emptyRes, errInvalidParentHash
			}

			if curBlockHeader.Number.Uint64() != blockNumber {
				log.Error("Mis-match in block number",
					"remoteBlockNumber", curBlockHeader.Number.Uint64(), "receivedBlockNumber", blockNumber)
				return emptyRes, errInvalidBlockNumber
			}
		}
		return work, nil
	case err := <-errc:
		return emptyRes, err
	}
}

// SubmitWork can be used by external miner to submit their POW solution.
// It returns an indication if the work was accepted.
// Note either an invalid solution, a stale work a non-existent work will return false.
func (api *API) SubmitWork(nonce types.BlockNonce, hash, digest common.Hash) bool {
	if api.ethash.remote == nil {
		return false
	}

	var blsSignature *BlsSignatureBytes

	var errc = make(chan error, 1)
	select {
	case api.ethash.remote.submitWorkCh <- &mineResult{
		nonce:     nonce,
		mixDigest: digest,
		hash:      hash,
		blsSeal:   blsSignature,
		errc:      errc,
	}:
	case <-api.ethash.remote.exitCh:
		return false
	}
	err := <-errc
	if err != nil {
		log.Error("SubmitWork: found error while submitting work", "error", err)
	}
	return err == nil
}

// SubmitWorkBLS can be used by external miner to submit their POS solution.
// It returns an indication if the work was accepted.
// Note either an invalid solution, a stale work a non-existent work will return false.
// This submit work contains BLS storing feature.
func (api *API) SubmitWorkBLS(nonce types.BlockNonce, hash common.Hash, hexSignatureString string) bool {
	if api.ethash.remote == nil {
		return false
	}

	signatureBytes := hexutil.MustDecode(hexSignatureString)
	blsSignatureBytes := new(BlsSignatureBytes)
	copy(blsSignatureBytes[:], signatureBytes[:])

	var errc = make(chan error, 1)

	select {
	case api.ethash.remote.submitWorkCh <- &mineResult{
		nonce:     nonce,
		mixDigest: common.BytesToHash(blsSignatureBytes[:32]),
		hash:      hash,
		blsSeal:   blsSignatureBytes,
		errc:      errc,
	}:
	case <-api.ethash.remote.exitCh:
		return false
	}
	err := <-errc
	return err == nil
}

// SubmitHashrate can be used for remote miners to submit their hash rate.
// This enables the node to report the combined hash rate of all miners
// which submit work through this node.
//
// It accepts the miner hash rate and an identifier which must be unique
// between nodes.
func (api *API) SubmitHashRate(rate hexutil.Uint64, id common.Hash) bool {
	if api.ethash.remote == nil {
		return false
	}

	var done = make(chan struct{}, 1)
	select {
	case api.ethash.remote.submitRateCh <- &hashrate{done: done, rate: uint64(rate), id: id}:
	case <-api.ethash.remote.exitCh:
		return false
	}

	// Block until hash rate submitted successfully.
	<-done
	return true
}

// GetHashrate returns the current hashrate for local CPU miner and remote miner.
func (api *API) GetHashrate() uint64 {
	return uint64(api.ethash.Hashrate())
}
