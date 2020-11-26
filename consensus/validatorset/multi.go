package validatorset

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
	"sort"
)

type Multi struct {
	sets map[int]ValidatorSet
}


func NewMulti(setMap map[int]ValidatorSet) *Multi {
	return &Multi{
		sets: setMap,
	}
}

func (multi *Multi) correctSet(blockNumber *big.Int) (ValidatorSet, int64) {
	if len(multi.sets) == 0 {
		log.Error("constructor validation ensures that there is at least one validator set for block 0; block 0 is less than any uint;")
		panic("constructor validation ensures that there is at least one validator set for block 0")
	}

	setNumbers := make([]int, 0)
	for key, _ := range multi.sets {
		setNumbers = append(setNumbers, key)
	}
	sort.Slice(setNumbers, func(i, j int) bool {
		return setNumbers[i] > setNumbers[j]
	})

	setNum := 0
	for _, setNumber := range setNumbers {
		if blockNumber.Cmp(big.NewInt(int64(setNumber))) >= 0 {
			setNum = setNumber
			break
		}
	}
	log.Debug("Multi ValidatorSet retrieved for block", "blockHeight", setNum)
	return multi.sets[setNum], int64(setNum)
}

func (multi *Multi) SignalToChange(first bool, receipts types.Receipts, header *types.Header, chain *core.BlockChain, chainDb ethdb.Database) ([]common.Address, bool, bool) {
	validator, setBlockNumber := multi.correctSet(header.Number)
	first = big.NewInt(setBlockNumber).Cmp(header.Number) == 0

	log.Debug("signal to change", "current validator", validator, "blockNum", header.Number)
	return validator.SignalToChange(first, receipts, header, chain, chainDb)
}

func (multi *Multi) FinalizeChange(header *types.Header, state *state.StateDB) error {
	validator, _ := multi.correctSet(header.Number)
	return validator.FinalizeChange(header, state)
}

func (multi *Multi) GetValidatorsByCaller(blockNumber *big.Int) []common.Address {
	validator, _ := multi.correctSet(blockNumber)
	log.Info("Current validator set ", "set", validator, "blockNumber", blockNumber)
	return validator.GetValidatorsByCaller(blockNumber)
}

func (multi *Multi) CountValidators() int {
	panic("implement me")
}

func (multi *Multi) PrepareBackend(header *types.Header, chain *core.BlockChain, chainDb ethdb.Database) error {
	validator, _ := multi.correctSet(header.Number)
	return validator.PrepareBackend(header, chain, chainDb)
}