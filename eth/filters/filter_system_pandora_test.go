/*
* Generated: 5/11/21
* This file is generated to support Lukso pandora module.
* Purpose:
 */
package filters

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
)

// GetPendingHeadsSince implements testBackend only for dummy purpose. So that existing code can run without an issue
func (b *testBackend) GetPendingHeadsSince(ctx context.Context, from common.Hash) []*types.Header {
	return nil
}

// SubscribePendingHeaderEvent implements testBackend only for dummy purpose. So that existing code can run without an issue
func (b *testBackend) SubscribePendingHeaderEvent(ch chan<- core.PendingHeaderEvent) event.Subscription {
	return b.pendingHeaderFeed.Subscribe(ch)
}

// TestPendingHeaderSubscription tests pending header events. In pending header, a batch of headers are come in insert header.
// pending header event send that batch to the API end.
func TestPendingHeaderSubscription(t *testing.T) {
	t.Parallel()

	// Initialize the backend
	var (
		db                  = rawdb.NewMemoryDatabase()
		backend             = &testBackend{db: db}
		api                 = NewPublicFilterAPI(backend, false, deadline)
		genesis             = new(core.Genesis).MustCommit(db)
		chain, _            = core.GenerateChain(params.TestChainConfig, genesis, ethash.NewFaker(), db, 10, func(i int, gen *core.BlockGen) {})
		pendingHeaderEvents = []core.PendingHeaderEvent{}
	)

	var headers []*types.Header

	// form the header chain from the created blocks
	for _, blk := range chain {
		headers = append(headers, blk.Header())
	}
	pendingHeaderEvents = append(pendingHeaderEvents, core.PendingHeaderEvent{Headers: headers})

	// create two subscriber channels
	chan0 := make(chan *types.Header)
	sub0 := api.events.SubscribePendingHeads(chan0)
	chan1 := make(chan *types.Header)
	sub1 := api.events.SubscribePendingHeads(chan1)

	go func() { // simulate client
		sub0Iterator, sub1Iterator := 0, 0
		// a batch of headers are received as event.
		// but in subscriber end we have to send the batch as one by one header
		for sub0Iterator != len(pendingHeaderEvents[0].Headers) || sub1Iterator != len(pendingHeaderEvents[0].Headers) {
			select {
			// here we will receive a single header from the batch and will process it
			case header := <-chan0:
				if pendingHeaderEvents[0].Headers[sub0Iterator].Hash() != header.Hash() {
					t.Errorf("sub0 received invalid hash on index %d, want %x, got %x", sub0Iterator, pendingHeaderEvents[0].Headers[sub0Iterator].Hash(), header.Hash())
				}
				sub0Iterator++
			case header := <-chan1:
				if pendingHeaderEvents[0].Headers[sub1Iterator].Hash() != header.Hash() {
					t.Errorf("sub1 received invalid hash on index %d, want %x, got %x", sub1Iterator, pendingHeaderEvents[0].Headers[sub1Iterator].Hash(), header.Hash())
				}
				sub1Iterator++
			}
		}

		sub0.Unsubscribe()
		sub1.Unsubscribe()
	}()

	time.Sleep(1 * time.Second)
	for _, e := range pendingHeaderEvents {
		// send the pending header batch to the feed.
		backend.pendingHeaderFeed.Send(e)
	}

	<-sub0.Err()
	<-sub1.Err()
}
