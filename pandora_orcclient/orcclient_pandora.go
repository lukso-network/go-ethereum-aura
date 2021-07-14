/*
* Generated: 5/31/21
* This file is generated to support Lukso pandora module.
* Purpose: All orchestrator client related codes reside here. Pandora use this client to talk with orchestrator.
 */
package pandora_orcclient

import (
	"context"

	"github.com/ethereum/go-ethereum/rpc"
)

// OrcClient defines typed wrappers for the Orchestrator RPC API.
type OrcClient struct {
	rpcClient *rpc.Client
}

// Dial connects a client to the given URL.
func Dial(rawurl string) (*OrcClient, error) {
	return DialContext(context.Background(), rawurl)
}

func DialContext(ctx context.Context, rawurl string) (*OrcClient, error) {
	client, err := rpc.DialContext(ctx, rawurl)
	if err != nil {
		return nil, err
	}
	return NewOrcClient(client), nil
}

// NewOrcClient creates an orchestrator client that uses the given RPC client.
func NewOrcClient(c *rpc.Client) *OrcClient {
	return &OrcClient{c}
}

// Close closes the orchestrator client
func (oc *OrcClient) Close() {
	oc.rpcClient.Close()
}

// GetConfirmedPanBlockHashes gets confirmation of pandora block hashes
func (oc *OrcClient) GetConfirmedPanBlockHashes(ctx context.Context, request []*BlockHash) ([]*BlockStatus, error) {
	var blockStatus []*BlockStatus
	if len(request) < 1 {
		// no request. dont do anything
		return blockStatus, nil
	}
	err := oc.rpcClient.CallContext(ctx, &blockStatus, "orc_confirmPanBlockHashes", request)
	return blockStatus, err
}
