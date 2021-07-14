/*
* Generated: 5/31/21
* This file is generated to support Lukso pandora module.
* Purpose:
 */
package pandora_orcclient

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// In process rpc server related test

// TestOrcClient_GetConfirmedPanBlockHashes tests in process pandora block confirmation
func TestOrcClient_GetConfirmedPanBlockHashes(t *testing.T) {
	orchestrator := DialInProcRPCClient()
	var request []*BlockHash
	request = append(request, &BlockHash{Hash: common.HexToHash(MockedHashInvalid), Slot: 1}, &BlockHash{Hash: common.HexToHash(MockedHashPending), Slot: 2})
	response, err := orchestrator.GetConfirmedPanBlockHashes(context.Background(), request)
	if err != nil {
		t.Fatalf("error found while getting confirmed pending block hashes")
	}
	t.Log("received block confirmations from orchestrator")
	for _, hash := range response {
		t.Logf("received %v", hash)
	}
}
