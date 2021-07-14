package core

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
)

/*
* Generated: May - 05 - 2021
* This file is generated to support Lukso pandora module
* Purpose: In response of https://github.com/lukso-network/pandora-execution-engine/issues/57 we need to have a pending
* in memory database. Which will hold the headers when they are locally validated but not validated by orchestrator node.
* Insert Headers operation will halt until the header is validated by orchestrator.
 */

// PandoraPendingHeaderContainer will hold temporary headers in a in memory db.
type PandoraPendingHeaderContainer struct {
	headerContainer ethdb.Database // in-memory database which will hold headers temporarily
	pndHeaderFeed   event.Feed     // announce new arrival of pending header
}

// WriteAndNotifyHeader writes header in database and also notify to the subscribers
func (container *PandoraPendingHeaderContainer) WriteAndNotifyHeader(header *types.Header) {
	// first write into database
	container.WriteHeader(header)

	// now send notification
	container.pndHeaderFeed.Send(PendingHeaderEvent{Headers: []*types.Header{header}})
}

// NewPandoraPendingHeaderContainer will return a fully initiated in-memory header container
func NewPandoraPendingHeaderContainer() *PandoraPendingHeaderContainer {
	return &PandoraPendingHeaderContainer{
		headerContainer: rawdb.NewMemoryDatabase(),
	}
}

// WriteHeaderBatch dumps a batch of header into header container
func (container *PandoraPendingHeaderContainer) WriteHeaderBatch(headers []*types.Header) {
	for _, header := range headers {
		container.WriteHeader(header)
	}
}

// WriteHeader dump a single header in the header container
func (container *PandoraPendingHeaderContainer) WriteHeader(header *types.Header) {
	// write the header into db
	rawdb.WriteHeader(container.headerContainer, header)

	// make the header as the top of the container queue. It will help us to get the last pushed header instance
	rawdb.WriteHeadHeaderHash(container.headerContainer, header.Hash())
}

// DeleteHeader deletes a single header from the container
func (container *PandoraPendingHeaderContainer) DeleteHeader(header *types.Header) {
	rawdb.DeleteHeader(container.headerContainer, header.Hash(), header.Number.Uint64())
}

// ReadHeaderSince will receive a from header hash and return a batch of headers from that header.
func (container *PandoraPendingHeaderContainer) ReadHeaderSince(from common.Hash) []*types.Header {
	fromHeaderNumber := rawdb.ReadHeaderNumber(container.headerContainer, from)

	lastHeaderNumber := rawdb.ReadHeaderNumber(container.headerContainer, rawdb.ReadHeadHeaderHash(container.headerContainer))

	var headers []*types.Header
	if fromHeaderNumber == nil {
		// fromHeaderNumber can be found nil in two cases:
		// 1. When requesting for empty hash. That is when orchestrator bootup it sends empty hash to the pandora. It is not present in the memory container
		// 2. When orchestrator requesting a from hash, which is already confirmed and removed from the memory container.
		// In both cases we are sending all available headers to the subscriber.
		return container.ReadAllHeaders()
	}

	if lastHeaderNumber == nil {
		// if lastHeaderNumber is nil then return immediately
		return headers
	}

	// for normal cases read blocks and return them
	for i := *fromHeaderNumber; i <= *lastHeaderNumber; i++ {
		header := container.readHeader(i)
		if header != nil {
			headers = append(headers, header)
		}
	}
	return headers
}

// readHeader reads a single header which is given as the header number
func (container *PandoraPendingHeaderContainer) readHeader(headerNumber uint64) *types.Header {
	hashes := rawdb.ReadAllHashes(container.headerContainer, headerNumber)
	if len(hashes) == 0 {
		// hash not found. so we can't read the header.
		return nil
	}
	return rawdb.ReadHeader(container.headerContainer, hashes[0], headerNumber)
}

// ReadAllHeaders reads all the headers from the memory
func (container *PandoraPendingHeaderContainer) ReadAllHeaders() []*types.Header {

	// first retrieve the hashes of the headers
	it := container.headerContainer.NewIterator([]byte("h"), nil)
	var headers []*types.Header
	for it.Next() {
		headerHash := common.BytesToHash(it.Key())
		headerNumber := rawdb.ReadHeaderNumber(container.headerContainer, headerHash)
		if headerNumber == nil {
			// if we get headerHash from the database then there must be the headernumber.
			// if we don't get header number then return error.

			return headers
		}
		headers = append(headers, rawdb.ReadHeader(container.headerContainer, headerHash, *headerNumber))
	}

	return headers
}
