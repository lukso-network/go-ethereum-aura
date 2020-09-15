package bindings

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"testing"
)

func TestNewParityChainSpec(t *testing.T) {
	parityFixture, err := ioutil.ReadFile("./fixtures/block-0-parity.json")
	assert.Nil(t, err)

	//	 Other stuff is not needed, I guess hash is really what matters for now
	//   If you want to strict compare you can compare indented bytes instead
	blockStruct := struct {
		Hash string `json:"hash"`
	}{}

	err = json.Unmarshal(parityFixture, &blockStruct)
	assert.Nil(t, err)

	parityGenesis, err := ioutil.ReadFile("./fixtures/parity-aura.json")
	assert.Nil(t, err)
	var parityChainSpec ParityChainSpec
	err = json.Unmarshal(parityGenesis, &parityChainSpec)
	assert.Nil(t, err)

	t.Run("Genesis file should produce same block 0 that in parity", func(t *testing.T) {
		var genesisGeth core.Genesis
		gethGenesisFixture, err := ioutil.ReadFile("./fixtures/geth-aura.json")
		assert.Nil(t, err)
		err = json.Unmarshal(gethGenesisFixture, &genesisGeth)
		assert.Nil(t, err)
		spec, err := NewParityChainSpec("AuthorityRound", &genesisGeth, params.GoerliBootnodes)
		assert.Nil(t, err)
		assert.NotNil(t, spec.Genesis)
		assert.NotNil(t, spec.Name)
		assert.NotNil(t, spec.Accounts)
		assert.NotNil(t, spec.Engine)
		assert.NotNil(t, spec.Nodes)
		assert.NotNil(t, spec.Params)
		assert.NotNil(t, spec.Engine.AuthorityRound)
		chainSpec, err := json.Marshal(spec.Engine.AuthorityRound)
		assert.Equal(t, "", fmt.Sprintf("%s", chainSpec))
	})
}
