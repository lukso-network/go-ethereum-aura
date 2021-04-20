package ethash

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	common2 "github.com/silesiacoin/bls/common"
	"github.com/silesiacoin/bls/herumi"
	"github.com/silesiacoin/bls/testutil/require"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"math/big"
	mathRand "math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeReader struct {
	chainConfig *params.ChainConfig
}

func (f fakeReader) Config() *params.ChainConfig {
	return f.chainConfig
}

func (f fakeReader) CurrentHeader() *types.Header {
	panic("implement me")
}

func (f fakeReader) GetHeader(hash common.Hash, number uint64) *types.Header {
	return &types.Header{Number: big.NewInt(1)}
}

func (f fakeReader) GetHeaderByNumber(number uint64) *types.Header {
	panic("implement me")
}

func (f fakeReader) GetHeaderByHash(hash common.Hash) *types.Header {
	panic("implement me")
}

type safeGatheredInfo struct {
	mutex    sync.Mutex
	gathered []*filters.MinimalEpochConsensusInfoPayload
}

func (info *safeGatheredInfo) appendNext(minimal *filters.MinimalEpochConsensusInfoPayload) {
	info.mutex.Lock()
	defer info.mutex.Unlock()
	info.gathered = append(info.gathered, minimal)
}

var chainHeaderReader consensus.ChainHeaderReader = fakeReader{
	chainConfig: &params.ChainConfig{
		ChainID:             big.NewInt(256),
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		BerlinBlock:         big.NewInt(0),
		SilesiaBlock:        big.NewInt(0),
	},
}

func TestPandora_SubscribeToMinimalConsensusInformation(t *testing.T) {
	timeNow := time.Now()
	epochDuration := pandoraEpochLength * time.Duration(SlotTimeDuration) * time.Second
	// Set genesis time in a past
	epochsProgressed := 3
	genesisTime := timeNow.Add(-epochDuration*time.Duration(epochsProgressed) + time.Duration(12)*time.Second)
	validatorPublicList := [pandoraEpochLength]common2.PublicKey{}

	for index, _ := range validatorPublicList {
		privKey, err := herumi.RandKey()
		assert.Nil(t, err)
		pubKey := privKey.PublicKey()
		validatorPublicList[index] = pubKey
	}

	minimalConsensusInfos := make([]*params.MinimalEpochConsensusInfo, 0)

	// Prepare epochs from the past
	for index := 0; index < epochsProgressed; index++ {
		consensusInfo := NewMinimalConsensusInfo(uint64(index)).(*MinimalEpochConsensusInfo)
		consensusInfo.AssignEpochStartFromGenesis(genesisTime)
		consensusInfo.AssignValidators(validatorPublicList)
		consensusInfoParam := &params.MinimalEpochConsensusInfo{
			Epoch:            consensusInfo.Epoch,
			ValidatorsList:   consensusInfo.ValidatorsList,
			EpochTimeStart:   consensusInfo.EpochTimeStartUnix,
			SlotTimeDuration: consensusInfo.SlotTimeDuration,
		}

		minimalConsensusInfos = append(minimalConsensusInfos, consensusInfoParam)
	}

	listener, server, location := makeOrchestratorServer(t, minimalConsensusInfos)
	defer server.Stop()
	require.Equal(t, location, listener.Addr().String())

	urls := []string{location}
	config := Config{
		PowMode: ModePandora,
		Log:     log.Root(),
	}

	var (
		consensusInfo []*params.MinimalEpochConsensusInfo
	)

	// Dummy genesis epoch
	genesisEpoch := &params.MinimalEpochConsensusInfo{Epoch: 0, ValidatorsList: validatorPublicList}
	consensusInfo = append(consensusInfo, genesisEpoch)
	ethash := NewPandora(config, urls, true, consensusInfo)
	remoteSealerServer := StartRemotePandora(ethash, urls, true)
	pandora := Pandora{remoteSealerServer}

	t.Run("temporary skipped", func(t *testing.T) {
		subscription, channel, err, errChannel := pandora.SubscribeToMinimalConsensusInformation(0)
		require.NoError(t, err)
		defer subscription.Unsubscribe()
		gatheredInformation := make([]*filters.MinimalEpochConsensusInfoPayload, 0)
		mutex := sync.Mutex{}
		gatherer := safeGatheredInfo{
			mutex:    mutex,
			gathered: gatheredInformation,
		}

		for {
			select {
			case minimalConsensus := <-channel:
				fmt.Printf("\n I have received consensus info \n")
				gatherer.appendNext(minimalConsensus)
			case err := <-errChannel:
				require.NoError(t, err)
			}

			if len(gatherer.gathered) >= epochsProgressed {
				assert.Equal(t, epochsProgressed, len(gatherer.gathered))
				require.NoError(t, err)
				testRetry := 5

				header := &types.Header{Time: uint64(timeNow.Unix())}

				var (
					currentEpoch *MinimalEpochConsensusInfo
				)

				// Try to test cache filling with decent fallback mechanism
				// cache propagation and networking could take some time
				for index := 0; index < testRetry; index++ {
					currentEpoch, err = ethash.getMinimalConsensus(header)

					if nil != err {
						time.Sleep(time.Millisecond * 50)

						continue
					}
				}

				require.NoError(t, err)
				assert.Equal(t, epochsProgressed, currentEpoch)
				fmt.Printf("\n I have it \n")

				return
			}
		}
	})
}

// This file is used for exploration of possible ways to achieve pandora-vanguard block production
// Test RemoteSigner approach connected to each other
func TestCreateBlockByPandoraAndVanguard(t *testing.T) {
	lruCache := newlru("cache", 12, newCache)
	lruDataset := newlru("dataset", 12, newDataset)

	privKey, err := herumi.RandKey()
	assert.Nil(t, err)

	pubKey := privKey.PublicKey()

	workSubmittedLock := sync.WaitGroup{}
	workSubmittedLock.Add(1)

	// Start a simple web vanguardServer to capture notifications.
	workChannel := make(chan [4]string)
	submitWorkChannel := make(chan *mineResult)

	// This is used to mimic server on vanguard which will consume pandora
	vanguardServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		blob, err := ioutil.ReadAll(req.Body)
		if err != nil {
			t.Errorf("failed to read miner notification: %v", err)
		}

		var work [4]string

		if err := json.Unmarshal(blob, &work); err != nil {
			t.Errorf("failed to unmarshal miner notification: %v", err)
		}

		workChannel <- work
		rlpHexHeader := work[2]
		rlpHeader, err := hexutil.Decode(rlpHexHeader)

		if nil != err {
			t.Errorf("failed to encode hex header")
		}

		signerFunc := func() {
			header := types.Header{}
			err = rlp.DecodeBytes(rlpHeader, &header)

			if nil != err {
				t.Errorf("failed to cast header as rlp")
			}

			// Motivation: you should always be sure that what you sign is valid.
			// We sign hash
			signatureBytes, err := hexutil.Decode(work[0])
			assert.Nil(t, err)
			signature := privKey.Sign(signatureBytes)

			isValidSignature := signature.Verify(pubKey, signatureBytes)

			if !isValidSignature {
				t.Errorf("Invalid signature received")
			}

			blsSignatureBytes := BlsSignatureBytes{}
			copy(blsSignatureBytes[:], signature.Marshal())

			//TODO: consider if it is needed
			header.MixDigest = common.BytesToHash(blsSignatureBytes[:])

			// Cast to []byte from [32]byte. This should prevent cropping
			workSubmittedLock.Done()

			// This in test is using work channel to push sealed block to pandora back
			submitWorkChannel <- &mineResult{
				nonce:     types.BlockNonce{},
				mixDigest: header.MixDigest,
				hash:      common.HexToHash(work[0]),
				blsSeal:   &blsSignatureBytes,
				errc:      nil,
			}
		}

		signerFunc()
	}))
	defer vanguardServer.Close()

	// This is how ethash would be designed to serve vanguard
	ethash := Ethash{
		caches:   lruCache,
		datasets: lruDataset,
		config: Config{
			// In pandora-vanguard implementation we do not need to increase nonce and mixHash is sealed/calculated on the Vanguard side
			PowMode: ModePandora,
			Log:     log.Root(),
		},
		lock:      sync.Mutex{},
		closeOnce: sync.Once{},
	}
	defer func() {
		_ = ethash.Close()
	}()
	urls := make([]string, 0)
	urls = append(urls, vanguardServer.URL)
	remoteSealerServer := StartRemotePandora(&ethash, urls, true)
	ethash.remote = remoteSealerServer
	ethashAPI := API{ethash: &ethash}

	header := &types.Header{
		ParentHash:  common.Hash{},
		UncleHash:   common.Hash{},
		Coinbase:    common.Address{},
		Root:        common.Hash{},
		TxHash:      common.Hash{},
		ReceiptHash: common.Hash{},
		Difficulty:  big.NewInt(1),
		Number:      big.NewInt(1),
		GasLimit:    0,
		GasUsed:     0,
		Time:        uint64(time.Now().UnixNano()),
		Extra:       nil,
		Nonce:       types.BlockNonce{},
	}

	t.Run("Should make work and notify vanguard", func(t *testing.T) {
		block := types.NewBlockWithHeader(header)
		results := make(chan *types.Block)
		err := ethash.Seal(nil, block, results, nil)
		assert.Nil(t, err)

		select {
		case work := <-workChannel:
			t.Run("Should have encodable sealHash", func(t *testing.T) {
				sealHash := ethash.SealHash(header).Hex()
				assert.Equal(t, sealHash, work[0])
			})

			t.Run("Should have encodable receiptHash", func(t *testing.T) {
				receiptHash := header.ReceiptHash
				assert.Equal(t, receiptHash.Hex(), work[1])
			})

			t.Run("Should have encodable rlp header", func(t *testing.T) {
				rlpHexHeader := work[2]
				rlpHeader, err := hexutil.Decode(rlpHexHeader)

				if nil != err {
					t.Errorf("failed to encode hex header")
				}

				header := types.Header{}
				err = rlp.DecodeBytes(rlpHeader, &header)

				if nil != err {
					t.Errorf("failed to cast header as rlp")
				}
			})

			t.Run("Should have encodable block number", func(t *testing.T) {
				assert.Equal(t, hexutil.Encode(header.Number.Bytes()), work[3])
			})

		//	Whole procedure should take no more than 1/2 of a slot. Lets test that for 6s / 4 ~ 2s
		case <-time.After(2 * time.Second):
			t.Fatalf("notification timed out")
		}

		// Wait until work is submitted back
		workSubmittedLock.Wait()

		select {
		// This is created by channel to remove network complexity for test scenario.
		// Full E2E layer should be somewhere else, or we should consider stress test
		case submittedWork := <-submitWorkChannel:
			submitted := ethashAPI.SubmitWorkBLS(
				submittedWork.nonce,
				submittedWork.hash,
				hexutil.Encode(submittedWork.blsSeal[:]),
			)

			blsSignatureBytes := submittedWork.blsSeal
			blsSignature, err := herumi.SignatureFromBytes(blsSignatureBytes[:])
			assert.Nil(t, err)
			assert.NotNil(t, blsSignature)

			hash := submittedWork.hash
			hashBytes := hash.Bytes()
			signatureValid := blsSignature.Verify(pubKey, hashBytes)
			assert.True(t, signatureValid)

			// This will return false, if anything goes wrong
			// TODO: debug why non-blocking result channel returns false
			// see: `https://gobyexample.com/non-blocking-channel-operations`
			//  Catch it at: consensus/ethash/sealer.go:447
			assert.False(t, submitted)
		case <-time.After(2 * time.Second):
			t.Fatalf("notification timed out")
		}
	})
}

func TestEthash_IsMinimalConsensusPresentForTime(t *testing.T) {
	lruCache := newlru("cache", 12, newCache)
	lruDataset := newlru("dataset", 12, newDataset)
	lruEpochSet := newlru("epochSet", 12, NewMinimalConsensusInfo)

	ethash := Ethash{
		caches:   lruCache,
		datasets: lruDataset,
		mci:      lruEpochSet,
		config: Config{
			// In pandora-vanguard implementation we do not need to increase
			// nonce and mixHash are sealed/calculated on the Vanguard side
			PowMode: ModePandora,
			Log:     log.Root(),
		},
		lock:      sync.Mutex{},
		closeOnce: sync.Once{},
	}
	defer func() {
		_ = ethash.Close()
	}()

	genesisEpoch := NewMinimalConsensusInfo(0).(*MinimalEpochConsensusInfo)
	genesisStart := time.Now()

	genesisEpoch.AssignEpochStartFromGenesis(genesisStart)
	assert.NoError(t, ethash.InsertMinimalConsensusInfo(0, genesisEpoch))

	// Time within an epoch
	timeWithinAnEpoch := uint64(genesisStart.Add(time.Duration(1)).Unix())

	// Very low unixTime
	timeBeforeEpoch := uint64(1)
	// Very high unix time
	timeAfterEpoch := uint64(genesisStart.Add(time.Duration(6856585) * time.Millisecond).Unix())

	assert.True(t, ethash.IsMinimalConsensusPresentForTime(uint64(genesisStart.Unix())))
	assert.True(t, ethash.IsMinimalConsensusPresentForTime(timeWithinAnEpoch))
	assert.False(t, ethash.IsMinimalConsensusPresentForTime(timeBeforeEpoch))
	assert.False(t, ethash.IsMinimalConsensusPresentForTime(timeAfterEpoch))
}

func TestEthash_Prepare_Pandora(t *testing.T) {
	lruCache := newlru("cache", 12, newCache)
	lruDataset := newlru("dataset", 12, newDataset)
	lruEpochSet := newlru("epochSet", 12, NewMinimalConsensusInfo)

	ethash := Ethash{
		caches:   lruCache,
		datasets: lruDataset,
		mci:      lruEpochSet,
		config: Config{
			// In pandora-vanguard implementation we do not need to increase
			// nonce and mixHash are sealed/calculated on the Vanguard side
			PowMode: ModePandora,
			Log:     log.Root(),
		},
		lock:      sync.Mutex{},
		closeOnce: sync.Once{},
	}
	defer func() {
		_ = ethash.Close()
	}()

	genesisEpoch := NewMinimalConsensusInfo(0).(*MinimalEpochConsensusInfo)
	genesisStart := time.Now()

	validatorPublicList := [validatorListLen]common2.PublicKey{}
	validatorPrivateList := [validatorListLen]common2.SecretKey{}

	for index, _ := range validatorPrivateList {
		privKey, err := herumi.RandKey()
		assert.Nil(t, err)
		pubKey := privKey.PublicKey()
		validatorPublicList[index] = pubKey
		validatorPrivateList[index] = privKey
	}

	genesisEpoch.AssignEpochStartFromGenesis(genesisStart)
	genesisEpoch.AssignValidators(validatorPublicList)

	assert.NoError(t, ethash.InsertMinimalConsensusInfo(0, genesisEpoch))
	genesisFromCache, genesisFetched := lruEpochSet.cache.Get(0)
	assert.True(t, genesisFetched)
	minimalConsensusFromCache := genesisFromCache.(*MinimalEpochConsensusInfo)
	assert.NotEqual(t, genesisEpoch.ValidatorsList[0], minimalConsensusFromCache.ValidatorsList[0])

	header := &types.Header{
		ParentHash:  common.Hash{},
		UncleHash:   common.Hash{},
		Coinbase:    common.Address{},
		Root:        common.Hash{},
		TxHash:      common.Hash{},
		ReceiptHash: common.Hash{},
		Bloom:       types.Bloom{},
		Difficulty:  nil,
		Number:      big.NewInt(0),
		GasLimit:    0,
		GasUsed:     0,
		Time:        uint64(time.Now().Unix()),
		Extra:       nil,
		MixDigest:   common.Hash{},
		Nonce:       types.BlockNonce{},
	}

	err := ethash.Prepare(chainHeaderReader, header)
	assert.Nil(t, err)

	expectedData, err := NewPandoraExtraData(header, genesisEpoch)
	assert.Nil(t, err)

	expectedDataBytes, err := rlp.EncodeToBytes(expectedData)
	assert.Nil(t, err)

	assert.Equal(t, expectedDataBytes, header.Extra)

	// InsertMinimalConsensusInfo function is temporary, this piece of code is just test for its behaviour
	t.Run("Should insert minimal consensus info", func(t *testing.T) {
		t.Run("Should return error when not in pandora mode", func(t *testing.T) {
			ethashNoPandora := &Ethash{}
			assert.Error(t, ethashNoPandora.InsertMinimalConsensusInfo(0, nil))
		})

		t.Run("Should insert consensus infos into cache", func(t *testing.T) {
			for i := 0; i < 8; i++ {
				index := i + 1
				timePassed := uint64(index)*uint64(time.Duration(SlotTimeDuration))*uint64(len(validatorPublicList)) + uint64(genesisStart.Unix())
				minimalConsensusInterface := NewMinimalConsensusInfo(uint64(index))
				minimalConsensus := minimalConsensusInterface.(*MinimalEpochConsensusInfo)
				minimalConsensus.EpochTimeStart = time.Unix(int64(timePassed), 0)
				minimalConsensus.EpochTimeStartUnix = timePassed
				minimalConsensus.AssignValidators(validatorPublicList)
				err = ethash.InsertMinimalConsensusInfo(uint64(index), minimalConsensus)
				assert.NoError(t, err)

				headerTime := timePassed + 6
				consensusFromCache, err := ethash.getMinimalConsensus(&types.Header{Time: headerTime})
				assert.NoError(t, err)
				assert.Equal(t, consensusFromCache.Epoch, uint64(index))
				require.DeepEqual(t, minimalConsensus, consensusFromCache)
			}
		})
	})
}

func TestAPI_InsertMinimalConsensusInfo(t *testing.T) {
	validatorHexStr := "0x9035868e41619fa5ee9fc502f8b021559aac9a14e675f8bc101658be9975cbbaf6d67afbc847dc4171b9f4c32550c43a"
	validatorHex := hexutil.MustDecode(validatorHexStr)
	validator, err := herumi.PublicKeyFromBytes(validatorHex)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"0x9035868e41619fa5ee9fc502f8b021559aac9a14e675f8bc101658be9975cbbaf6d67afbc847dc4171b9f4c32550c43a",
		hexutil.Encode(validator.Marshal()),
	)
}

func TestMinimalEpochConsensusInfo_AssignEpochStartFromGenesis(t *testing.T) {
	maxIterations := 2 ^ 7
	genesisTime := time.Now()

	for i := 0; i < maxIterations; i++ {
		minimalEpochConsensusInfo := NewMinimalConsensusInfo(uint64(i))
		consensusInfo := minimalEpochConsensusInfo.(*MinimalEpochConsensusInfo)
		consensusInfo.AssignEpochStartFromGenesis(genesisTime)
		epochTimeStart := consensusInfo.EpochTimeStart
		seconds := time.Duration(SlotTimeDuration) * time.Second * time.Duration(i) * time.Duration(validatorListLen)
		expectedEpochTime := genesisTime.Add(seconds)
		assert.Equal(t, expectedEpochTime.Unix(), epochTimeStart.Unix())
	}
}

func TestVerifySeal(t *testing.T) {
	lruCache := newlru("cache", 12, newCache)
	lruDataset := newlru("dataset", 12, newDataset)
	lruEpochSet := newlru("epochSet", 12, NewMinimalConsensusInfo)
	validatorPublicList := [validatorListLen]common2.PublicKey{}
	validatorPrivateList := [validatorListLen]common2.SecretKey{}

	for index, _ := range validatorPrivateList {
		privKey, err := herumi.RandKey()
		assert.Nil(t, err)
		pubKey := privKey.PublicKey()
		validatorPublicList[index] = pubKey
		validatorPrivateList[index] = privKey
	}

	genesisEpoch := NewMinimalConsensusInfo(0).(*MinimalEpochConsensusInfo)
	genesisStart := time.Now()
	genesisEpoch.AssignEpochStartFromGenesis(genesisStart)
	genesisEpoch.AssignValidators(validatorPublicList)
	// Should not be evicted
	assert.False(t, lruEpochSet.cache.Add(0, genesisEpoch))
	assert.True(t, lruEpochSet.cache.Contains(0))
	assert.False(t, lruEpochSet.cache.Add(1, genesisEpoch))
	assert.True(t, lruEpochSet.cache.Contains(1))
	// Check epochs from cache
	genesisEpochFromCache, ok := lruEpochSet.cache.Get(0)
	assert.Equal(t, genesisEpochFromCache, genesisEpoch)
	assert.True(t, ok)
	nextEpochSet, ok := lruEpochSet.cache.Get(1)
	assert.Equal(t, nextEpochSet, genesisEpoch)
	assert.True(t, ok)

	// This is how ethash would be designed to serve vanguard
	ethash := Ethash{
		caches:   lruCache,
		datasets: lruDataset,
		mci:      lruEpochSet,
		config: Config{
			// In pandora-vanguard implementation we do not need to increase nonce and mixHash is sealed/calculated on the Vanguard side
			PowMode: ModePandora,
			Log:     log.Root(),
		},
		lock:      sync.Mutex{},
		closeOnce: sync.Once{},
	}
	defer func() {
		_ = ethash.Close()
	}()

	t.Run("Should increment over slots in first epoch", func(t *testing.T) {
		headers := make([]*types.Header, 0)
		for index, privateKey := range validatorPrivateList {
			headerTime := genesisEpoch.EpochTimeStart.Add(SlotTimeDuration * time.Second * time.Duration(index))
			// Add additional second to not be on start of the slot
			randMin := 0
			randMax := 5
			randomInterval := mathRand.Intn(randMax-randMin) + randMin
			headerTime = headerTime.Add(time.Second * time.Duration(randomInterval))
			extraData := &PandoraExtraData{
				Slot:  uint64(index),
				Epoch: uint64(0),
				Turn:  uint64(index),
			}
			header, _, _ := generatePandoraSealedHeaderByKey(privateKey, int64(index), headerTime, extraData)
			headers = append(headers, header)

			err := ethash.verifySeal(nil, header, false)

			assert.Nil(
				t,
				err,
				fmt.Sprintf("failed on index: %d, with headerExtra: %v", index, header.Extra),
			)
		}

		t.Run("Should fail before overflowing the validator set length", func(t *testing.T) {
			// Check next epoch
			nextEpochHeaderNumber := len(validatorPrivateList) + 1
			headerTime := genesisEpoch.EpochTimeStart.Add(
				SlotTimeDuration * time.Second * time.Duration(nextEpochHeaderNumber))
			header, _, _ := generatePandoraSealedHeaderByKey(
				validatorPrivateList[0],
				int64(nextEpochHeaderNumber),
				headerTime,
				// This is checked at last so can be empty
				&PandoraExtraData{},
			)
			assert.Error(
				t,
				ethash.verifySeal(nil, header, false),
				fmt.Sprintf("should fail on index: %d, with header: %v", nextEpochHeaderNumber, header),
			)
		})
	})

	t.Run("Should discard invalid slot sealer in second epoch", func(t *testing.T) {
		headers := make([]*types.Header, 0)
		privateKey, err := herumi.RandKey()
		assert.Nil(t, err)

		for index, _ := range validatorPrivateList {
			headerTime := genesisEpoch.EpochTimeStart.Add(SlotTimeDuration * time.Second * time.Duration(index))
			// Add additional second to not be on start of the slot
			randMin := 0
			randMax := 5
			randomInterval := mathRand.Intn(randMax-randMin) + randMin
			headerTime = headerTime.Add(time.Second * time.Duration(randomInterval))
			header, sealHash, blsSignature := generatePandoraSealedHeaderByKey(
				privateKey,
				int64(index),
				headerTime,
				&PandoraExtraData{},
			)
			headers = append(headers, header)

			expectedErr := fmt.Errorf(
				"invalid signature: %s in header hash: %s with sealHash: %s",
				blsSignature.Marshal(),
				header.Hash().String(),
				sealHash.String(),
			)
			assert.Equal(
				t,
				expectedErr,
				ethash.verifySeal(nil, header, false),
			)
		}
	})
}

func generatePandoraSealedHeaderByKey(
	privKey common2.SecretKey,
	headerNumber int64,
	headerTime time.Time,
	extraData *PandoraExtraData,
) (
	header *types.Header,
	headerHash common.Hash,
	blsSignature common2.Signature,
) {
	ethash := NewTester(nil, true)
	extraDataBytes, _ := rlp.EncodeToBytes(extraData)

	header = &types.Header{
		ParentHash:  common.Hash{},
		UncleHash:   common.Hash{},
		Coinbase:    common.Address{},
		Root:        common.Hash{},
		TxHash:      common.Hash{},
		ReceiptHash: common.Hash{},
		Bloom:       types.Bloom{},
		Difficulty:  big.NewInt(1),
		Number:      big.NewInt(headerNumber),
		GasLimit:    0,
		GasUsed:     0,
		Time:        uint64(headerTime.Unix()),
		Extra:       extraDataBytes,
		MixDigest:   common.Hash{},
		Nonce:       types.BlockNonce{},
	}

	headerHash = ethash.SealHash(header)
	signature := privKey.Sign(headerHash.Bytes())

	extraDataWithSignature := new(PandoraExtraDataSealed)
	extraDataWithSignature.FromExtraDataAndSignature(*extraData, signature)
	verified := signature.Verify(privKey.PublicKey(), headerHash.Bytes())

	if !verified {
		panic("Signature should be valid")
	}

	compressedSignature := signature.Marshal()[:herumi.CompressedSize]
	header.MixDigest = common.BytesToHash(compressedSignature[:])
	blsSignature = signature

	extraDataBytes, err := rlp.EncodeToBytes(extraDataWithSignature)

	if nil != err {
		panic(err.Error())
	}

	header.Extra = extraDataBytes

	return
}

type testBackend struct {
	mux             *event.TypeMux
	db              ethdb.Database
	sections        uint64
	txFeed          event.Feed
	logsFeed        event.Feed
	rmLogsFeed      event.Feed
	pendingLogsFeed event.Feed
	chainFeed       event.Feed
}

func (b *testBackend) ChainDb() ethdb.Database {
	return b.db
}

func (b *testBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error) {
	var (
		hash common.Hash
		num  uint64
	)
	if blockNr == rpc.LatestBlockNumber {
		hash = rawdb.ReadHeadBlockHash(b.db)
		number := rawdb.ReadHeaderNumber(b.db, hash)
		if number == nil {
			return nil, nil
		}
		num = *number
	} else {
		num = uint64(blockNr)
		hash = rawdb.ReadCanonicalHash(b.db, num)
	}
	return rawdb.ReadHeader(b.db, hash, num), nil
}

func (b *testBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	number := rawdb.ReadHeaderNumber(b.db, hash)
	if number == nil {
		return nil, nil
	}
	return rawdb.ReadHeader(b.db, hash, *number), nil
}

func (b *testBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	if number := rawdb.ReadHeaderNumber(b.db, hash); number != nil {
		return rawdb.ReadReceipts(b.db, hash, *number, params.TestChainConfig), nil
	}
	return nil, nil
}

func (b *testBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	number := rawdb.ReadHeaderNumber(b.db, hash)
	if number == nil {
		return nil, nil
	}
	receipts := rawdb.ReadReceipts(b.db, hash, *number, params.TestChainConfig)

	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (b *testBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.txFeed.Subscribe(ch)
}

func (b *testBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.rmLogsFeed.Subscribe(ch)
}

func (b *testBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.logsFeed.Subscribe(ch)
}

func (b *testBackend) SubscribePendingLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.pendingLogsFeed.Subscribe(ch)
}

func (b *testBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.chainFeed.Subscribe(ch)
}

func (b *testBackend) BloomStatus() (uint64, uint64) {
	return params.BloomBitsBlocks, b.sections
}

func (b *testBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	requests := make(chan chan *bloombits.Retrieval)

	go session.Multiplex(16, 0, requests)
	go func() {
		for {
			// Wait for a service request or a shutdown
			select {
			case <-ctx.Done():
				return

			case request := <-requests:
				task := <-request

				task.Bitsets = make([][]byte, len(task.Sections))
				//for i, section := range task.Sections {
				//if rand.Int()%4 != 0 { // Handle occasional missing deliveries
				//	head := rawdb.ReadCanonicalHash(b.db, (section+1)*params.BloomBitsBlocks-1)
				//	task.Bitsets[i], _ = rawdb.ReadBloomBits(b.db, task.Bit, section, head)
				//}
				//}
				request <- task
			}
		}
	}()
}

func makeOrchestratorServer(
	t *testing.T,
	minimalConsensusInfo []*params.MinimalEpochConsensusInfo,
) (listener net.Listener, server *rpc.Server, location string) {
	location = "./test.ipc"
	apis := make([]rpc.API, 0)
	deadline := 5 * time.Minute
	api := filters.NewPublicFilterAPI(&testBackend{}, false, deadline)
	api.ConsensusInfo = minimalConsensusInfo

	apis = append(apis, rpc.API{
		Namespace: "orc",
		Version:   "1.0",
		Service:   api,
		Public:    true,
	})

	listener, server, err := rpc.StartIPCEndpoint(location, apis)
	require.NoError(t, err)

	return
}
