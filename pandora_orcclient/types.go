package pandora_orcclient

import "github.com/ethereum/go-ethereum/common"

type Status string

const (
	Pending  Status = "Pending"
	Verified Status = "Verified"
	Invalid  Status = "Invalid"
	Skipped  Status = "Skipped"
)

type BlockHash struct {
	Slot uint64
	Hash common.Hash
}

type BlockStatus struct {
	BlockHash
	Status Status
}
