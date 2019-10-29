package explorerdb

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/threefoldtech/rivine/modules"
	"github.com/threefoldtech/rivine/persist"
	"github.com/threefoldtech/rivine/pkg/encoding/rivbin"
	"github.com/threefoldtech/rivine/types"

	"github.com/asdine/storm"
	bolt "go.etcd.io/bbolt"
)

type StormDB struct {
	db       *storm.DB
	logger   *persist.Logger
	bcInfo   types.BlockchainInfo
	chainCts types.ChainConstants
}

func NewStormDB(path string, bcInfo types.BlockchainInfo, chainCts types.ChainConstants, verbose bool) (*StormDB, error) {
	db, err := storm.Open(filepath.Join(path, "explorer.db"), storm.Codec(rivbinMarshalUnmarshaler{}))
	if err != nil {
		return nil, err
	}
	// Initialize the logger.
	logFilePath := filepath.Join(path, "explorer.log")
	logger, err := persist.NewFileLogger(bcInfo, logFilePath, verbose)
	if err != nil {
		return nil, err
	}
	return &StormDB{
		db:       db,
		logger:   logger,
		chainCts: chainCts,
	}, nil
}

var (
	bucketNameMetadata              = []byte("Metadata")
	metadataKeyChainContext         = []byte("ChainContext")
	metadataKeyInternalData         = []byte("InternalData")
	metadataKeyChainAggregatedFacts = []byte("ChainAggregatedFacts")
)

const (
	nodeNamePublicKeys = "PublicKeys"
	nodeNameBlocks     = "Blocks"

	nodePublicKeysKeyUnlockHash = "UnlockHash"

	nodeBlocksKeyReference = "Reference"
)

type rivbinMarshalUnmarshaler struct{}

func (rb rivbinMarshalUnmarshaler) Marshal(v interface{}) ([]byte, error) {
	// do not marshal ptrs, this function is only called on root objects, so should be fine
	// ... last famous words o.o
	val := reflect.ValueOf(v)
	if val.IsValid() && val.Kind() == reflect.Ptr && !val.IsNil() {
		val = val.Elem()
	}
	v = val.Interface()
	return rivbin.Marshal(v)
}
func (rb rivbinMarshalUnmarshaler) Unmarshal(b []byte, v interface{}) error {
	return rivbin.Unmarshal(b, v)
}
func (rb rivbinMarshalUnmarshaler) Name() string {
	return "rivbin"
}

// used for block rp -> bID node
type blockReferencePointIDPair struct {
	Reference ReferencePoint `storm:"id"`
	BlockID   types.BlockID
}

// used for unlockHash -> PublicKey node
type unlockHashPublicKeyPair struct {
	UnlockHash types.UnlockHash `storm:"id"`
	PublicKey  types.PublicKey
}

type stormDBInternalData struct {
	LastDataID stormDataID
}

// used for intermediate collections of updates to a specific unlock hash
type (
	unlockHashUpdateCollection struct {
		updates   map[types.UnlockHash]*unlockHashUpdate
		block     types.BlockID
		height    types.BlockHeight
		timestamp types.Timestamp
	}

	unlockHashUpdate struct {
		coinInputs        []*Output
		coinOutputs       []*Output
		blockStakeInputs  []*Output
		blockStakeOutputs []*Output

		transactions    map[types.TransactionID]struct{}
		linkedAddresses map[types.UnlockHash]struct{}

		unlockedCoinOutputs       []*stormOutput
		unlockedBlockStakeOutputs []*stormOutput
	}
)

func newUnlockHashUpdateCollection(block types.BlockID, height types.BlockHeight, timestamp types.Timestamp) *unlockHashUpdateCollection {
	return &unlockHashUpdateCollection{
		updates:   make(map[types.UnlockHash]*unlockHashUpdate),
		block:     block,
		height:    height,
		timestamp: timestamp,
	}
}

func (uhUpdate *unlockHashUpdate) Transactions() (txns []types.TransactionID) {
	for txnID := range uhUpdate.transactions {
		txns = append(txns, txnID)
	}
	return
}

func (uhUpdate *unlockHashUpdate) LinkedAddresses() (addresses []types.UnlockHash) {
	for uh := range uhUpdate.linkedAddresses {
		addresses = append(addresses, uh)
	}
	return
}

func (uhuc *unlockHashUpdateCollection) getUpdateForUnlockHash(uh types.UnlockHash) *unlockHashUpdate {
	uhUpdate, ok := uhuc.updates[uh]
	if ok {
		return uhUpdate
	}
	uhUpdate = &unlockHashUpdate{
		transactions: make(map[types.TransactionID]struct{}),
	}
	uhuc.updates[uh] = uhUpdate
	return uhUpdate
}

func referenceParentHashAsTransactionIfNeeded(uhUpdate *unlockHashUpdate, output *Output) {
	if output.Type == OutputTypeBlockStake || output.Type == OutputTypeCoin {
		uhUpdate.transactions[types.TransactionID(output.ParentID)] = struct{}{}
	}
}

func (uhuc *unlockHashUpdateCollection) linkUnlockhashToOthers(uh types.UnlockHash, condition types.UnlockConditionProxy) {
	var uhUpdate *unlockHashUpdate
	mscond := condition.Condition.(types.UnlockHashSliceGetter)
	for _, uh := range mscond.UnlockHashSlice() {
		uhUpdate = uhuc.getUpdateForUnlockHash(uh)
		uhUpdate.linkedAddresses[uh] = struct{}{}
	}
}

func (uhuc *unlockHashUpdateCollection) RegisterInput(input *Output) {
	uh := input.Condition.UnlockHash()
	uhUpdate := uhuc.getUpdateForUnlockHash(uh)

	registerInputForUnlockHashUpdate(uh, uhUpdate, input)

	if uh.Type == types.UnlockTypeMultiSig {
		uhuc.linkUnlockhashToOthers(uh, input.Condition)
	} else if uh.Type == types.UnlockTypeNil {
		pairs := RivineUnlockHashPublicKeyPairsFromFulfillment(input.SpenditureData.Fulfillment)
		for _, pair := range pairs {
			uhUpdate := uhuc.getUpdateForUnlockHash(pair.UnlockHash)
			registerInputForUnlockHashUpdate(pair.UnlockHash, uhUpdate, input)
		}
	}
}

func registerInputForUnlockHashUpdate(uh types.UnlockHash, uhUpdate *unlockHashUpdate, input *Output) {
	if input.Type == OutputTypeBlockStake {
		uhUpdate.blockStakeInputs = append(uhUpdate.blockStakeInputs, input)
	} else {
		uhUpdate.coinInputs = append(uhUpdate.coinInputs, input)
	}
	referenceParentHashAsTransactionIfNeeded(uhUpdate, input)
}

func (uhuc *unlockHashUpdateCollection) RegisterOutput(output *Output) {
	uh := output.Condition.UnlockHash()
	uhUpdate := uhuc.getUpdateForUnlockHash(uh)
	if output.Type == OutputTypeBlockStake {
		uhUpdate.blockStakeOutputs = append(uhUpdate.blockStakeOutputs, output)
	} else {
		uhUpdate.coinOutputs = append(uhUpdate.coinOutputs, output)
	}
	if uh.Type == types.UnlockTypeMultiSig {
		uhuc.linkUnlockhashToOthers(uh, output.Condition)
	}
	referenceParentHashAsTransactionIfNeeded(uhUpdate, output)
}

func (uhuc *unlockHashUpdateCollection) RegisterUnlockedOutputs(outputs []stormOutput) {
	var (
		uh       types.UnlockHash
		uhUpdate *unlockHashUpdate
		output   *stormOutput
	)
	for idx := range outputs {
		output = &outputs[idx]
		uh = output.Condition.UnlockHash()
		uhUpdate = uhuc.getUpdateForUnlockHash(uh)
		if output.Type == OutputTypeBlockStake {
			uhUpdate.unlockedBlockStakeOutputs = append(uhUpdate.unlockedBlockStakeOutputs, output)
		} else {
			uhUpdate.unlockedCoinOutputs = append(uhUpdate.unlockedCoinOutputs, output)
		}
	}
}

func (uhuc *unlockHashUpdateCollection) RegisterTransaction(uh types.UnlockHash, txid types.TransactionID) {
	uhUpdate := uhuc.getUpdateForUnlockHash(uh)
	uhUpdate.transactions[txid] = struct{}{}
}

// TODO: where we use manual bolt calls, re-use existing bolt.Tx, once we do start using those somehow

func (sdb *StormDB) saveInternalData(internalData stormDBInternalData) error {
	b, err := rivbin.Marshal(internalData)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal internal stormDB data %v: %v", internalData, err)
	}
	return sdb.db.Bolt.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketNameMetadata)
		if err != nil {
			return fmt.Errorf("bucket %s is not created while it is expected to be: %v", string(bucketNameMetadata), err)
		}
		return bucket.Put(metadataKeyInternalData, b)
	})
}

func (sdb *StormDB) getInternalData() (stormDBInternalData, error) {
	var internalDataBytes []byte
	err := sdb.db.Bolt.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketNameMetadata)
		if bucket == nil {
			return nil
		}
		internalDataBytes = bucket.Get(metadataKeyInternalData)
		return nil
	})
	if err != nil {
		return stormDBInternalData{}, err
	}
	if len(internalDataBytes) == 0 { // start from zero
		return stormDBInternalData{
			LastDataID: 1, // start at index 1, as stormDB doesn't allow a 0-index
		}, nil
	}
	var internalData stormDBInternalData
	err = rivbin.Unmarshal(internalDataBytes, &internalData)
	if err != nil {
		return stormDBInternalData{}, fmt.Errorf(
			"failed to unmarshal internal stormDB data %x: %v", internalDataBytes, err)
	}
	return internalData, nil
}

func (sdb *StormDB) SetChainContext(chainCtx ChainContext) error {
	b, err := rivbin.Marshal(chainCtx)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal chain context %v: %v", chainCtx, err)
	}
	return sdb.db.Bolt.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketNameMetadata)
		if err != nil {
			return fmt.Errorf("bucket %s is not created while it is expected to be: %v", string(bucketNameMetadata), err)
		}
		return bucket.Put(metadataKeyChainContext, b)
	})
}

func (sdb *StormDB) GetChainContext() (ChainContext, error) {
	var chainCtxBytes []byte
	err := sdb.db.Bolt.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketNameMetadata)
		if bucket == nil {
			return nil
		}
		chainCtxBytes = bucket.Get(metadataKeyChainContext)
		return nil
	})
	if err != nil {
		return ChainContext{}, err
	}
	if len(chainCtxBytes) == 0 { // start from zero
		return ChainContext{
			ConsensusChangeID: modules.ConsensusChangeBeginning,
		}, nil
	}
	var chainCtx ChainContext
	err = rivbin.Unmarshal(chainCtxBytes, &chainCtx)
	if err != nil {
		return ChainContext{}, fmt.Errorf(
			"failed to unmarshal chain context %x: %v", chainCtxBytes, err)
	}
	return chainCtx, nil
}

func (sdb *StormDB) GetChainAggregatedFacts() (ChainAggregatedFacts, error) {
	var factsBytes []byte
	err := sdb.db.Bolt.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketNameMetadata)
		if bucket == nil {
			return nil
		}
		factsBytes = bucket.Get(metadataKeyChainAggregatedFacts)
		return nil
	})
	if err != nil {
		return ChainAggregatedFacts{}, err
	}
	if len(factsBytes) == 0 { // start from zero
		return ChainAggregatedFacts{}, nil
	}
	var facts ChainAggregatedFacts
	err = rivbin.Unmarshal(factsBytes, &facts)
	if err != nil {
		return ChainAggregatedFacts{}, fmt.Errorf(
			"failed to unmarshal chain aggregated facts %x: %v", factsBytes, err)
	}
	return facts, nil
}

func (sdb *StormDB) updateChainAggregatedFacts(cb func(facts *ChainAggregatedFacts) error) error {
	return sdb.db.Bolt.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketNameMetadata)
		if err != nil {
			return err
		}

		var (
			facts      ChainAggregatedFacts
			factsBytes []byte
		)
		if factsBytes = bucket.Get(metadataKeyChainAggregatedFacts); len(factsBytes) != 0 {
			err = rivbin.Unmarshal(factsBytes, &facts)
			if err != nil {
				return fmt.Errorf(
					"failed to unmarshal chain aggregated facts %x: %v", factsBytes, err)
			}
		}

		err = cb(&facts)
		if err != nil {
			return err
		}

		factsBytes, err = rivbin.Marshal(facts)
		if err != nil {
			return err
		}

		return bucket.Put(metadataKeyChainAggregatedFacts, factsBytes)
	})
}

func (sdb *StormDB) ApplyBlock(block Block, blockFacts BlockFactsConstants, txs []Transaction, outputs []Output, inputs map[types.OutputID]OutputSpenditureData) error {
	sdbInternalData, err := sdb.getInternalData()
	if err != nil {
		return err
	}

	node := newStormObjectNodeReaderWriter(sdb.db, sdbInternalData.LastDataID)
	blockNode := sdb.db.From(nodeNameBlocks)
	publicKeysNode := sdb.db.From(nodeNamePublicKeys)

	// used to update wallets and contracts
	uhUpdateCollection := newUnlockHashUpdateCollection(block.ID, block.Height, block.Timestamp)

	// store block reference point parings
	err = blockNode.Save(&blockReferencePointIDPair{
		Reference: ReferencePoint(block.Height) + 1, // require +1, as a storm identifier cannot be 0
		BlockID:   block.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to apply block: failed to save block %s's height %d as reference point: %v", block.ID.String(), block.Height, err)
	}
	err = blockNode.Save(&blockReferencePointIDPair{
		Reference: ReferencePoint(block.Timestamp),
		BlockID:   block.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to apply block: failed to save block %s's timestamp %d as reference point: %v", block.ID.String(), block.Timestamp, err)
	}
	// store transactions
	for idx, tx := range txs {
		err = node.SaveTransaction(&tx)
		if err != nil {
			return fmt.Errorf(
				"failed to apply block: failed to save txn %s (#%d) of block %s: %v",
				tx.ID.String(), idx+1, block.ID.String(), err)
		}
		// get the common extension data and save its information as well
		extensionData, err := tx.GetCommonExtensionData()
		if err != nil {
			return fmt.Errorf(
				"failed to apply block: failed to get extension data from txn %s (#%d) of block %s: %v",
				tx.ID.String(), idx+1, block.ID.String(), err)
		}
		// store all fulfillments and link it to the unlockhashes
		for _, fulfillment := range extensionData.Fulfillments {
			pairs := RivineUnlockHashPublicKeyPairsFromFulfillment(fulfillment)
			// store public keys
			for _, pair := range pairs {
				err = publicKeysNode.Save(&unlockHashPublicKeyPair{
					UnlockHash: pair.UnlockHash,
					PublicKey:  pair.PublicKey,
				})
				if err != nil {
					return fmt.Errorf("failed to apply block: failed to save block %s's unlock hash %s mapped to public key %s: %v", block.ID.String(), pair.UnlockHash.String(), pair.PublicKey.String(), err)
				}
			}
		}
		// link conditions to the unlockhashes
		for _, condition := range extensionData.Conditions {
			uhUpdateCollection.RegisterTransaction(condition.UnlockHash(), tx.ID)
		}
	}
	// store outputs
	for idx, output := range outputs {
		err = node.SaveOutput(&output)
		if err != nil {
			return fmt.Errorf(
				"failed to apply block: failed to save output %s (#%d) of parent %s: %v",
				output.ID.String(), idx+1, output.ParentID.String(), err)
		}
		uhUpdateCollection.RegisterOutput(&output)
	}
	// store inputs
	inputOutputSlice := make([]*Output, 0, len(inputs))
	for outputID, spenditureData := range inputs {
		output, err := node.UpdateOutputSpenditureData(outputID, &spenditureData)
		if err != nil {
			return fmt.Errorf(
				"failed to apply block %s: failed to update output (as spent): failed to update existing output %s: %v",
				block.ID.String(), outputID.String(), err)
		}
		inputOutputSlice = append(inputOutputSlice, &output)
		uhUpdateCollection.RegisterInput(&output)
	}
	// update aggregated facts
	facts, outputsUnlocked, err := sdb.applyBlockToAggregatedFacts(block.Height, block.Timestamp, node, outputs, inputOutputSlice, blockFacts.Target)
	if err != nil {
		return fmt.Errorf("failed to apply block: failed to save aggregated chain facts at block %s (height: %d): %v", block.ID.String(), block.Height, err)
	}
	// keep track of unlocked outputs for the unlock hash updates
	uhUpdateCollection.RegisterUnlockedOutputs(outputsUnlocked)

	// store block with facts at the end, now that we have the final chain facts, after applying this block
	err = node.SaveBlockWithFacts(&block, &BlockFacts{
		Constants: blockFacts,
		Aggregated: BlockFactsAggregated{
			TotalCoins:                 facts.TotalCoins,
			TotalLockedCoins:           facts.TotalLockedCoins,
			TotalBlockStakes:           facts.TotalBlockStakes,
			TotalLockedBlockStakes:     facts.TotalLockedBlockStakes,
			EstimatedActiveBlockStakes: facts.EstimatedActiveBlockStakes,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to apply block %s: failed to save block %s with facts: %v", block.ID.String(), err)
	}

	// store all updates to unlockhashes
	err = sdb.applyUnlockHashUpdates(node, uhUpdateCollection)
	if err != nil {
		return fmt.Errorf("failed to apply block %s: failed to save unlock hash updates: %v", block.ID.String(), err)
	}

	// finally store the internal stormDB data
	sdbInternalData.LastDataID = node.GetLastDataID()
	return sdb.saveInternalData(sdbInternalData)
}

func (sdb *StormDB) applyBlockToAggregatedFacts(height types.BlockHeight, timestamp types.Timestamp, objectNode stormObjectNodeReaderWriter, outputs []Output, inputs []*Output, target types.Target) (ChainAggregatedFacts, []stormOutput, error) {
	var (
		err             error
		isLocked        bool
		outputsUnlocked []stormOutput
	)
	// get outputs unlocked by height and timestamp
	if height != 0 { // do not do it for block 0, as it will return all outputs that do not have a lock
		outputsUnlocked, err = objectNode.GetStormOutputsbyUnlockReferencePoint(height, timestamp)
		if err != nil {
			if err == storm.ErrNotFound {
				// ignore not found
				outputsUnlocked = nil
			} else {
				return ChainAggregatedFacts{}, nil, fmt.Errorf("failed to get outputs unlocked by height %d or timestamp %d: %v", height, timestamp, err)
			}
		}
	}
	// get outputs unlocked by timestamp
	var resultFacts ChainAggregatedFacts
	err = sdb.updateChainAggregatedFacts(func(facts *ChainAggregatedFacts) error {
		// discount all new inputs from total coins/blockstakes
		for _, input := range inputs {
			if input.Type == OutputTypeBlockStake {
				facts.TotalBlockStakes = facts.TotalBlockStakes.Sub(input.Value)
			} else {
				facts.TotalCoins = facts.TotalCoins.Sub(input.Value)
			}
		}
		// count all new outputs, and if locked also add them to the total locked assets
		for _, output := range outputs {
			isLocked = output.UnlockReferencePoint > 0 && !output.UnlockReferencePoint.Overreached(height, timestamp)
			if output.Type == OutputTypeBlockStake {
				facts.TotalBlockStakes = facts.TotalBlockStakes.Add(output.Value)
				if isLocked {
					facts.TotalLockedBlockStakes = facts.TotalLockedBlockStakes.Add(output.Value)
				}
			} else {
				facts.TotalCoins = facts.TotalCoins.Add(output.Value)
				if isLocked {
					facts.TotalLockedCoins = facts.TotalLockedCoins.Add(output.Value)
				}
			}
		}
		// discount all unlocked outputs from total locked coins/blockstakes
		for _, output := range outputsUnlocked {
			if output.Type == OutputTypeBlockStake {
				facts.TotalLockedBlockStakes = facts.TotalLockedBlockStakes.Sub(output.Value)
			} else {
				facts.TotalLockedCoins = facts.TotalLockedCoins.Sub(output.Value)
			}
		}
		// update estimated active block stakes
		facts.AddLastBlockContext(BlockFactsContext{
			Target:    target,
			Timestamp: timestamp,
		})
		facts.EstimatedActiveBlockStakes = sdb.estimateActiveBS(height, timestamp, facts.LastBlocks)

		// keep a shallow copy of the facts
		resultFacts = *facts
		return nil
	})
	return resultFacts, outputsUnlocked, err
}

func (sdb *StormDB) applyUnlockHashUpdates(node stormObjectNodeReaderWriter, uhUpdateCollection *unlockHashUpdateCollection) error {
	var err error
	for uh, uhUpdate := range uhUpdateCollection.updates {
		switch uh.Type {
		case types.UnlockTypeNil:
			err = sdb.applyFreeForAllWalletUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		case types.UnlockTypePubKey:
			err = sdb.applySingleSignatureWalletUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		case types.UnlockTypeMultiSig:
			err = sdb.applyMultiSignatureWalletUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		case types.UnlockTypeAtomicSwap:
			err = sdb.applyAtomicSwapContractUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		default: // TODO: support extension types
			sdb.logger.Printf("[WARN] applyUnlockHashUpdates: cannot apply update for unknown unlockhash %s", uh.String())
		}
	}
	// all updated fine
	return nil
}

func applyBaseWalletUpdate(blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, wallet *WalletData, uhUpdate *unlockHashUpdate) error {
	// update the wallet
	if wallet.UnlockHash.Type == types.UnlockTypeNil {
		// start from a fresh contract, thus register it
		wallet.UnlockHash = uh
	}

	var err error

	// update block information
	wallet.Blocks = append(wallet.Blocks, blockID)

	// update transaction info
	wallet.Transactions = append(wallet.Transactions, uhUpdate.Transactions()...)

	// update coin information
	for _, ci := range uhUpdate.coinInputs {
		err = wallet.CoinBalance.ApplyInput(ci)
		if err != nil {
			return fmt.Errorf("failed to update wallet %s: failed to apply coin input %s: %v", uh.String(), ci.ID.String(), err)
		}
	}
	for _, co := range uhUpdate.coinOutputs {
		err = wallet.CoinBalance.ApplyOutput(height, timestamp, co)
		if err != nil {
			return fmt.Errorf("failed to update wallet %s: failed to apply coin output %s: %v", uh.String(), co.ID.String(), err)
		}
		// only here we need to add coin output ID info, as it is already known for coin inputs because what we do here
		wallet.CoinOutputs = append(wallet.CoinOutputs, co.ID)
	}
	for _, co := range uhUpdate.unlockedCoinOutputs {
		output := co.AsOutput(types.OutputID{}) // that output ID is unknown here is not important
		err = wallet.CoinBalance.ApplyUnlockedOutput(height, timestamp, &output)
		if err != nil {
			return fmt.Errorf("failed to update wallet %s: failed to apply unlocked coin output with dataID %d: %v", uh.String(), co.DataID, err)
		}
	}

	//  update block stake information
	for _, ci := range uhUpdate.blockStakeInputs {
		err = wallet.BlockStakeBalance.ApplyInput(ci)
		if err != nil {
			return fmt.Errorf("failed to update wallet %s: failed to apply block stake input %s: %v", uh.String(), ci.ID.String(), err)
		}
	}
	for _, co := range uhUpdate.blockStakeOutputs {
		err = wallet.BlockStakeBalance.ApplyOutput(height, timestamp, co)
		if err != nil {
			return fmt.Errorf("failed to update wallet %s: failed to apply block stake output %s: %v", uh.String(), co.ID.String(), err)
		}
		// only here we need to add block stake output ID info, as it is already known for block stake inputs because what we do here
		wallet.BlockStakeOutputs = append(wallet.BlockStakeOutputs, co.ID)
	}
	for _, bso := range uhUpdate.unlockedBlockStakeOutputs {
		output := bso.AsOutput(types.OutputID{}) // that output ID is unknown here is not important
		err = wallet.BlockStakeBalance.ApplyUnlockedOutput(height, timestamp, &output)
		if err != nil {
			return fmt.Errorf("failed to update wallet %s: failed to apply block stake output with dataID %d: %v", uh.String(), bso.DataID, err)
		}
	}

	// all good, wallet update applied succesfully
	return nil
}

func (sdb *StormDB) applyFreeForAllWalletUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	// get the wallet (or start from a fresh one if it is new)
	wallet, err := node.GetFreeForAllWallet(uh)
	if err != nil && err != storm.ErrNotFound {
		// return any error other than not found,
		// with not found errors we simply start with a fresh contract
		return fmt.Errorf("failed to update free-for-all wallet %s: failed to fetch it: %v", uh.String(), err)
	}

	// apply base update
	err = applyBaseWalletUpdate(blockID, height, timestamp, uh, &wallet.WalletData, uhUpdate)
	if err != nil {
		return fmt.Errorf("free-for-all wallet update apply error: %v", err)
	}

	// save the updated wallet
	err = node.SaveFreeForAllWallet(&wallet)
	if err != nil {
		return fmt.Errorf("failed to update free-for-all wallet %s: failed to store it: %v", uh.String(), err)
	}

	// all good, wallet saved succesfully
	return nil
}

func (sdb *StormDB) applySingleSignatureWalletUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	// get the wallet (or start from a fresh one if it is new)
	wallet, err := node.GetSingleSignatureWallet(uh)
	if err != nil && err != storm.ErrNotFound {
		// return any error other than not found,
		// with not found errors we simply start with a fresh contract
		return fmt.Errorf("failed to update single signature wallet %s: failed to fetch it: %v", uh.String(), err)
	}

	// apply base update
	err = applyBaseWalletUpdate(blockID, height, timestamp, uh, &wallet.WalletData, uhUpdate)
	if err != nil {
		return fmt.Errorf("single signature wallet update error: %v", err)
	}

	// apply single signature specific logic
	wallet.AddMultiSignatureWallets(uhUpdate.LinkedAddresses()...)

	// save the updated wallet
	err = node.SaveSingleSignatureWallet(&wallet)
	if err != nil {
		return fmt.Errorf("failed to update single signature wallet %s: failed to store it: %v", uh.String(), err)
	}

	// all good, wallet saved succesfully
	return nil
}

func (sdb *StormDB) applyMultiSignatureWalletUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	// get the wallet (or start from a fresh one if it is new)
	wallet, err := node.GetMultiSignatureWallet(uh)
	if err != nil && err != storm.ErrNotFound {
		// return any error other than not found,
		// with not found errors we simply start with a fresh contract
		return fmt.Errorf("failed to update multi signature wallet %s: failed to fetch it: %v", uh.String(), err)
	}

	// apply base update
	err = applyBaseWalletUpdate(blockID, height, timestamp, uh, &wallet.WalletData, uhUpdate)
	if err != nil {
		return fmt.Errorf("multi signature wallet update error: %v", err)
	}

	// keep track of owners and public keys, to update only where needed
	if wallet.RequiredSgnatureCount == 0 {
		assignOwnerUnlockHashesToWallet := func(cond types.UnlockConditionProxy) error {
			var marCond types.MarshalableUnlockCondition
			if ct := cond.ConditionType(); ct == types.ConditionTypeMultiSignature {
				marCond = cond.Condition
			} else if ct == types.ConditionTypeTimeLock {
				marCond = cond.Condition.(*types.TimeLockCondition).Condition
				if marCond.ConditionType() != types.ConditionTypeMultiSignature {
					return fmt.Errorf("failed to update multi signature wallet %s: unexpected TimeLock-wrapped condition of type %d referenced in block %s", uh.String(), ct, blockID.String())
				}
			} else {
				return fmt.Errorf("failed to update multi signature wallet %s: unexpected condition of type %d referenced in block %s", uh.String(), ct, blockID.String())
			}
			mscond := marCond.(types.MultiSignatureConditionOwnerInfoGetter)
			wallet.RequiredSgnatureCount = int(mscond.GetMinimumSignatureCount())
			uhSlice := mscond.UnlockHashSlice()
			wallet.Owners = make([]types.UnlockHash, 0, len(uhSlice))
			for _, uh := range uhSlice {
				wallet.Owners = append(wallet.Owners, uh)
			}
			// all good
			return nil
		}

		err = func() error { // return on first assignment, as none should fail, and only 1 is required
			for _, ci := range uhUpdate.coinInputs {
				return assignOwnerUnlockHashesToWallet(ci.Condition)
			}
			for _, bsi := range uhUpdate.blockStakeInputs {
				return assignOwnerUnlockHashesToWallet(bsi.Condition)
			}
			// shouldn't happen AFAIK, as a multi signature wallet is only created when a first input appears
			return fmt.Errorf("no coin- or block stake inputs found to create multi signature wallet %s owner information from", uh.String())
		}()
		if err != nil {
			return err
		}
	}

	// save the updated wallet
	err = node.SaveMultiSignatureWallet(&wallet)
	if err != nil {
		return fmt.Errorf("failed to update multi signature wallet %s: failed to store it: %v", uh.String(), err)
	}

	// all good, wallet saved succesfully
	return nil
}

func (sdb *StormDB) applyAtomicSwapContractUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	// get the contract (or start from a fresh one if it is new)
	contract, err := node.GetAtomicSwapContract(uh)
	if err != nil && err != storm.ErrNotFound {
		// return any error other than not found,
		// with not found errors we simply start with a fresh contract
		return fmt.Errorf("failed to update atomic swap contract %s: failed to fetch it: %v", uh.String(), err)
	}

	// update the contract
	var contractCreated bool
	if contract.UnlockHash.Type == types.UnlockTypeNil {
		// start from a fresh contract, thus register it
		contract.UnlockHash = uh
		contractCreated = true
		if col := len(uhUpdate.coinOutputs); col != 1 {
			return fmt.Errorf("failed to update atomic swap contract %s: invalid update info: coin output length has to be 1 but is %d: %v", uh.String(), col, err)
		}
		co := uhUpdate.coinOutputs[0]
		// update contract value
		contract.CoinInput = types.CoinOutputID(co.ID)
		contract.ContractValue = co.Value
		if ct := co.Condition.ConditionType(); ct != types.ConditionTypeAtomicSwap {
			return fmt.Errorf("failed to update atomic swap contract %s: invalid update info: unexpected atomic swap condition %d: %v", uh.String(), ct, err)
		}
		contract.ContractCondition = *(co.Condition.Condition.(*types.AtomicSwapCondition))
	}
	// update contract with spenditure data if info is available (required if not created now)
	if cil := len(uhUpdate.coinInputs); cil != 1 {
		// update spenditure info
		ci := uhUpdate.coinInputs[0]
		if ci.SpenditureData == nil {
			return fmt.Errorf("failed to update atomic swap contract %s: invalid update info: nil input (%s) spenditure: %v", uh.String(), ci.ID.String(), err)
		}
		if ft := ci.SpenditureData.Fulfillment.FulfillmentType(); ft != types.FulfillmentTypeAtomicSwap {
			return fmt.Errorf("failed to update atomic swap contract %s: invalid update info: unexpected atomic swap fulfillment %d: %v", uh.String(), ft, err)
		}
		contract.SpenditureData = &AtomicSwapContractSpenditureData{
			ContractFulfillment: *(ci.SpenditureData.Fulfillment.Fulfillment.(*types.AtomicSwapFulfillment)),
			CoinOutput:          types.CoinOutputID(ci.ID),
		}
	} else if cil != 0 {
		return fmt.Errorf("failed to update atomic swap contract %s: invalid update info: coin input length has to be 0 or 1 but is %d: %v", uh.String(), cil, err)
	} else if !contractCreated {
		return fmt.Errorf("failed to update atomic swap contract %s: invalid update info: contract was created in past but no new information received: %v", uh.String(), err)
	}
	// update transaction info
	if txl := len(uhUpdate.transactions); txl == 0 || txl > 2 {
		return fmt.Errorf("failed to update atomic swap contract %s: invalid update info: transaction length has to be 1 or 2 but is %d: %v", uh.String(), txl, err)
	}
	for txID := range uhUpdate.transactions {
		contract.Transactions = append(contract.Transactions, txID)
	}

	// save the updated contract
	err = node.SaveAtomicSwapContract(&contract)
	if err != nil {
		return fmt.Errorf("failed to update atomic swap contract %s: failed to store it: %v", uh.String(), err)
	}

	// all good, contract saved succesfully
	return nil
}

func (sdb *StormDB) estimateActiveBS(height types.BlockHeight, timestamp types.Timestamp, blocks []BlockFactsContext) types.Currency {
	if len(blocks) == 0 {
		return types.Currency{}
	}
	var (
		estimatedActiveBS types.Difficulty
		block             BlockFactsContext

		lBlockOffset    = len(blocks) - 1
		oldestTimestamp = blocks[lBlockOffset].Timestamp
		totalDifficulty = blocks[lBlockOffset].Target
	)
	for i := range blocks[:lBlockOffset] {
		block = blocks[lBlockOffset-i]
		totalDifficulty = totalDifficulty.AddDifficulties(block.Target, sdb.chainCts.RootDepth)
		oldestTimestamp = block.Timestamp
	}
	secondsPassed := timestamp - oldestTimestamp
	estimatedActiveBS = totalDifficulty.Difficulty(sdb.chainCts.RootDepth)
	if secondsPassed > 0 {
		estimatedActiveBS = estimatedActiveBS.Div64(uint64(secondsPassed))
	}
	return types.NewCurrency(estimatedActiveBS.Big())
}

func (sdb *StormDB) RevertBlock(blockContext BlockRevertContext, txs []types.TransactionID, outputs []types.OutputID, inputs []types.OutputID) error {
	sdbInternalData, err := sdb.getInternalData()
	if err != nil {
		return err
	}

	node := newStormObjectNodeReaderWriter(sdb.db, sdbInternalData.LastDataID)
	blockNode := sdb.db.From(nodeNameBlocks)

	// used to update wallets and contracts
	uhUpdateCollection := newUnlockHashUpdateCollection(
		blockContext.ID, blockContext.Height, blockContext.Timestamp)

	// delete block
	_, err = node.DeleteBlock(blockContext.ID)
	if err != nil {
		return fmt.Errorf("failed to revert block: failed to delete block %s by ID: %v", blockContext.ID.String(), err)
	}
	// delete block reference point parings
	err = blockNode.DeleteStruct(&blockReferencePointIDPair{
		Reference: ReferencePoint(blockContext.Height) + 1, // require +1, as a storm identifier cannot be 0
		BlockID:   blockContext.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to revert block: failed to delete block %s's height %d by reference point: %v", blockContext.ID.String(), blockContext.Height, err)
	}
	err = blockNode.Save(&blockReferencePointIDPair{
		Reference: ReferencePoint(blockContext.Timestamp),
		BlockID:   blockContext.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to revert block: failed to delete block %s's timestamp %d by reference point: %v", blockContext.ID.String(), blockContext.Timestamp, err)
	}
	// delete transactions
	for idx, txnID := range txs {
		txn, err := node.DeleteTransaction(txnID)
		if err != nil {
			return fmt.Errorf(
				"failed to revert block: failed to delete txn %s (#%d) of block %s by ID: %v",
				txnID.String(), idx+1, blockContext.ID.String(), err)
		}
		// get the common extension data and save its information as well
		extensionData, err := txn.GetCommonExtensionData()
		if err != nil {
			return fmt.Errorf(
				"failed to apply block: failed to get extension data from txn %s (#%d) of block %s: %v",
				txnID.String(), idx+1, blockContext.ID.String(), err)
		}
		// link conditions to the unlockhashes
		for _, condition := range extensionData.Conditions {
			uhUpdateCollection.RegisterTransaction(condition.UnlockHash(), txnID)
		}
	}
	// delete outputs
	outputSlice := make([]*Output, 0, len(outputs))
	for _, outputID := range outputs {
		output, err := node.DeleteOutput(outputID)
		if err != nil {
			return fmt.Errorf(
				"failed to revert block: failed to delete unspent output %s of block %s by ID: %v",
				outputID.String(), blockContext.ID.String(), err)
		}
		outputSlice = append(outputSlice, &output)
		uhUpdateCollection.RegisterOutput(&output)
	}
	// delete inputs
	inputOutputSlice := make([]*Output, 0, len(inputs))
	for _, inputID := range inputs {
		output, err := node.UpdateOutputSpenditureData(inputID, nil)
		if err != nil {
			return fmt.Errorf(
				"failed to revert block: failed to unmark spent output %s of block %s: failed to update existing output: %v",
				inputID.String(), blockContext.ID.String(), err)
		}
		inputOutputSlice = append(inputOutputSlice, &output)
		uhUpdateCollection.RegisterInput(&output)
	}
	// update aggregated facts
	updatesLocked, err := sdb.revertBlockToAggregatedFacts(blockContext.Height, blockContext.Timestamp, node, outputSlice, inputOutputSlice)
	if err != nil {
		return fmt.Errorf("failed to apply block: failed to save aggregated chain facts at block %s (height: %d): %v", blockContext.ID.String(), blockContext.Height, err)
	}
	// keep track of locked outputs for the unlock hash updates
	uhUpdateCollection.RegisterUnlockedOutputs(updatesLocked)

	// store all updates to unlockhashes
	err = sdb.revertUnlockHashUpdates(node, uhUpdateCollection)
	if err != nil {
		return fmt.Errorf("failed to revert block %s: failed to save unlock hash updates: %v", blockContext.ID.String(), err)
	}

	// finally store the internal stormDB data
	sdbInternalData.LastDataID = node.GetLastDataID()
	return sdb.saveInternalData(sdbInternalData)
}

func (sdb *StormDB) revertBlockToAggregatedFacts(height types.BlockHeight, timestamp types.Timestamp, objectNode stormObjectNodeReaderWriter, outputs []*Output, inputs []*Output) ([]stormOutput, error) {
	var (
		err           error
		isLocked      bool
		outputsLocked []stormOutput
	)
	// get outputs unlocked by height and timestamp
	if height != 0 { // do not do it for block 0, as it will return all outputs that do not have a lock
		outputsLocked, err = objectNode.GetStormOutputsbyUnlockReferencePoint(height, timestamp)
		if err != nil {
			if err == storm.ErrNotFound {
				// ignore not found
				outputsLocked = nil
			} else {
				return nil, fmt.Errorf("failed to get outputs locked until height %d or timestamp %d: %v", height, timestamp, err)
			}
		}
	}
	// get outputs unlocked by timestamp
	err = sdb.updateChainAggregatedFacts(func(facts *ChainAggregatedFacts) error {
		// re-apply all reverted inputs to total coins/blockstakes
		for _, input := range inputs {
			if input.Type == OutputTypeBlockStake {
				facts.TotalBlockStakes = facts.TotalBlockStakes.Add(input.Value)
			} else {
				facts.TotalCoins = facts.TotalCoins.Add(input.Value)
			}
		}
		// re-apply all locked outputs to total locked coins/blockstakes
		for _, output := range outputsLocked {
			if output.Type == OutputTypeBlockStake {
				facts.TotalLockedBlockStakes = facts.TotalLockedBlockStakes.Add(output.Value)
			} else {
				facts.TotalLockedCoins = facts.TotalLockedCoins.Add(output.Value)
			}
		}
		// discount all reverted outputs, and if locked also subtract them to the total locked assets
		for _, output := range outputs {
			isLocked = output.UnlockReferencePoint > 0 && !output.UnlockReferencePoint.Overreached(height, timestamp)
			if output.Type == OutputTypeBlockStake {
				facts.TotalBlockStakes = facts.TotalBlockStakes.Sub(output.Value)
				if isLocked {
					facts.TotalLockedBlockStakes = facts.TotalLockedBlockStakes.Sub(output.Value)
				}
			} else {
				facts.TotalCoins = facts.TotalCoins.Sub(output.Value)
				if isLocked {
					facts.TotalLockedCoins = facts.TotalLockedCoins.Sub(output.Value)
				}
			}
		}
		facts.RemoveLastBlockContext()
		facts.EstimatedActiveBlockStakes = sdb.estimateActiveBS(height, timestamp, facts.LastBlocks)
		// all good
		return nil
	})
	return outputsLocked, err
}

func (sdb *StormDB) revertUnlockHashUpdates(node stormObjectNodeReaderWriter, uhUpdateCollection *unlockHashUpdateCollection) error {
	var err error
	for uh, uhUpdate := range uhUpdateCollection.updates {
		switch uh.Type {
		case types.UnlockTypeNil:
			err = sdb.revertFreeForAllWalletUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		case types.UnlockTypePubKey:
			err = sdb.revertSingleSignatureWalletUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		case types.UnlockTypeMultiSig:
			err = sdb.revertMultiSignatureWalletUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		case types.UnlockTypeAtomicSwap:
			err = sdb.revertAtomicSwapContractUpdate(
				node,
				uhUpdateCollection.block,
				uhUpdateCollection.height,
				uhUpdateCollection.timestamp,
				uh, uhUpdate)
			if err != nil {
				return err
			}
		default: // TODO: support extension types
			sdb.logger.Printf("[WARN] revertUnlockHashUpdates: cannot revert update for unknown unlockhash %s", uh.String())
		}
	}
	// all updated fine
	return nil
}

func revertBaseWalletUpdate(blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, wallet *WalletData, uhUpdate *unlockHashUpdate) error {
	// revert block ID
	err := wallet.RevertBlock(blockID)
	if err != nil {
		return fmt.Errorf("failed to revert block %s from wallet %s: %v", blockID.String(), uh.String(), err)
	}

	// revert transactions
	err = wallet.RevertTransactions(uhUpdate.Transactions()...)
	if err != nil {
		return fmt.Errorf("failed to revert transactions from block %s from wallet %s: %v", blockID.String(), uh.String(), err)
	}

	// revert coin information
	for _, ci := range uhUpdate.coinInputs {
		err = wallet.CoinBalance.RevertInput(ci)
		if err != nil {
			return fmt.Errorf("failed to revert wallet %s: failed to revert coin input %s: %v", uh.String(), ci.ID.String(), err)
		}
	}
	for _, co := range uhUpdate.coinOutputs {
		err = wallet.CoinBalance.RevertOutput(height, timestamp, co)
		if err != nil {
			return fmt.Errorf("failed to revert wallet %s: failed to revert coin output %s: %v", uh.String(), co.ID.String(), err)
		}
		err = wallet.RevertCoinOutput(co.ID)
		if err != nil {
			return fmt.Errorf("failed to revert wallet %s: %v", uh.String(), err)
		}
	}
	for _, co := range uhUpdate.unlockedCoinOutputs {
		output := co.AsOutput(types.OutputID{}) // that output ID is unknown here is not important
		err = wallet.CoinBalance.RevertUnlockedOutput(height, timestamp, &output)
		if err != nil {
			return fmt.Errorf("failed to revert wallet %s: failed to revert unlocked coin output with dataID %d: %v", uh.String(), co.DataID, err)
		}
	}

	// revert block stake information
	for _, bsi := range uhUpdate.blockStakeInputs {
		err = wallet.BlockStakeBalance.RevertInput(bsi)
		if err != nil {
			return fmt.Errorf("failed to revert wallet %s: failed to revert block stake input %s: %v", uh.String(), bsi.ID.String(), err)
		}
	}
	for _, bso := range uhUpdate.blockStakeOutputs {
		err = wallet.BlockStakeBalance.RevertOutput(height, timestamp, bso)
		if err != nil {
			return fmt.Errorf("failed to revert wallet %s: failed to revert block stake output %s: %v", uh.String(), bso.ID.String(), err)
		}
		err = wallet.RevertBlockStakeOutput(bso.ID)
		if err != nil {
			return fmt.Errorf("failed to revert allet %s: %v", uh.String(), err)
		}
	}
	for _, bso := range uhUpdate.unlockedBlockStakeOutputs {
		output := bso.AsOutput(types.OutputID{}) // that output ID is unknown here is not important
		err = wallet.CoinBalance.RevertUnlockedOutput(height, timestamp, &output)
		if err != nil {
			return fmt.Errorf("failed to revert wallet %s: failed to revert unlocked block stake output with dataID %d: %v", uh.String(), bso.DataID, err)
		}
	}

	// all good, wallet update reverted succesfully
	return nil
}

func (sdb *StormDB) revertFreeForAllWalletUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	// get the wallet (or start from a fresh one if it is new)
	wallet, err := node.GetFreeForAllWallet(uh)
	if err != nil && err != storm.ErrNotFound {
		// return any error other than not found,
		// with not found errors we simply start with a fresh contract
		return fmt.Errorf("failed to revert block %s from free-for-all wallet %s: failed to fetch it: %v", blockID.String(), uh.String(), err)
	}

	// revert base update
	err = revertBaseWalletUpdate(blockID, height, timestamp, uh, &wallet.WalletData, uhUpdate)
	if err != nil {
		return fmt.Errorf("free-for-all wallet update revert error: %v", err)
	}

	// save the updated wallet
	err = node.SaveFreeForAllWallet(&wallet)
	if err != nil {
		return fmt.Errorf("failed to update free-for-all wallet %s: failed to store it: %v", uh.String(), err)
	}

	// all good, wallet saved succesfully
	return nil
}

func (sdb *StormDB) revertSingleSignatureWalletUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	// get the wallet (or start from a fresh one if it is new)
	wallet, err := node.GetSingleSignatureWallet(uh)
	if err != nil && err != storm.ErrNotFound {
		// return any error other than not found,
		// with not found errors we simply start with a fresh contract
		return fmt.Errorf("failed to revert block %s from single signature wallet %s: failed to fetch it: %v", blockID.String(), uh.String(), err)
	}

	// apply base update
	err = revertBaseWalletUpdate(blockID, height, timestamp, uh, &wallet.WalletData, uhUpdate)
	if err != nil {
		return fmt.Errorf("single signature wallet update error: %v", err)
	}

	// removing multi signature wallet references is never required,
	// as these addresses are deterministic and will thus always be true,
	// whether used or not is irrelevant, what is known is no longer unknown

	// save the updated wallet
	err = node.SaveSingleSignatureWallet(&wallet)
	if err != nil {
		return fmt.Errorf("failed to update single signature wallet %s: failed to store it: %v", uh.String(), err)
	}

	// all good, wallet saved succesfully
	return nil
}

func (sdb *StormDB) revertMultiSignatureWalletUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	// get the wallet (or start from a fresh one if it is new)
	wallet, err := node.GetMultiSignatureWallet(uh)
	if err != nil && err != storm.ErrNotFound {
		// return any error other than not found,
		// with not found errors we simply start with a fresh contract
		return fmt.Errorf("failed to revert block %s from multi signature wallet %s: failed to fetch it: %v", blockID.String(), uh.String(), err)
	}

	// apply base update
	err = revertBaseWalletUpdate(blockID, height, timestamp, uh, &wallet.WalletData, uhUpdate)
	if err != nil {
		return fmt.Errorf("multi signature wallet update error: %v", err)
	}

	// removing multi signature wallet owners is never required,
	// as these addresses are deterministic and will thus always be true,
	// whether used or not is irrelevant, what is known is no longer unknown

	// save the updated wallet
	err = node.SaveMultiSignatureWallet(&wallet)
	if err != nil {
		return fmt.Errorf("failed to update multi signature wallet %s: failed to store it: %v", uh.String(), err)
	}

	// all good, wallet saved succesfully
	return nil
}

func (sdb *StormDB) revertAtomicSwapContractUpdate(node stormObjectNodeReaderWriter, blockID types.BlockID, height types.BlockHeight, timestamp types.Timestamp, uh types.UnlockHash, uhUpdate *unlockHashUpdate) error {
	if len(uhUpdate.coinOutputs) == 1 {
		// contract can be deleted, as the coin output (containing the initial definition) is reverted
		err := node.DeleteAtomicSwapContract(uh)
		if err != nil {
			return fmt.Errorf("failed to delete atomic swap contract %s: %v", uh.String(), err)
		}
	}

	// only other revert possible is the spenditure revert
	if cil := len(uhUpdate.coinInputs); cil != 1 {
		return fmt.Errorf("failed to revert atomic swap contract %s spenditure: unexpected update coin input length of %d (expected 1)", uh.String(), cil)
	}
	txns := uhUpdate.Transactions()
	if txnl := len(txns); txnl != 1 {
		return fmt.Errorf("failed to revert atomic swap contract %s spenditure: unexpected update transaction length of %d (expected 2)", uh.String(), txnl)
	}

	// get the contract (or start from a fresh one if it is new)
	contract, err := node.GetAtomicSwapContract(uh)
	if err != nil { // ErrNotFound should be countedf as an error here
		return fmt.Errorf("failed to revert atomic swap contract %s spenditure: failed to fetch it: %v", uh.String(), err)
	}

	// remove the spenditure data of the contract
	if bytes.Compare(contract.Transactions[1][:], txns[0][:]) == 0 {
		contract.Transactions = contract.Transactions[:1]
	} else {
		contract.Transactions = contract.Transactions[1:2]
	}
	contract.SpenditureData = nil

	// save the updated contract
	err = node.SaveAtomicSwapContract(&contract)
	if err != nil {
		return fmt.Errorf("failed to revert atomic swap contract %s spenditure: failed to store it: %v", uh.String(), err)
	}

	// all good, contract saved succesfully
	return nil
}

func (sdb *StormDB) GetObject(id ObjectID) (Object, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetObject(id)
}

func (sdb *StormDB) GetObjectInfo(id ObjectID) (ObjectInfo, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetObjectInfo(id)
}

func (sdb *StormDB) GetBlock(id types.BlockID) (Block, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetBlock(id)
}

func (sdb *StormDB) GetBlockFacts(id types.BlockID) (BlockFacts, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetBlockFacts(id)
}

func (sdb *StormDB) GetBlockByReferencePoint(rp ReferencePoint) (Block, error) {
	blockID, err := sdb.GetBlockIDByReferencePoint(rp)
	if err != nil {
		return Block{}, nil
	}
	return sdb.GetBlock(blockID)
}

func (sdb *StormDB) GetBlockIDByReferencePoint(rp ReferencePoint) (types.BlockID, error) {
	node := sdb.db.From(nodeNameBlocks)
	var pair blockReferencePointIDPair
	if rp.IsBlockHeight() {
		rp++
	}
	err := node.One(nodeBlocksKeyReference, rp, &pair)
	if err != nil {
		return types.BlockID{}, err
	}
	return pair.BlockID, nil
}

func (sdb *StormDB) GetBlockFactsByReferencePoint(rp ReferencePoint) (BlockFacts, error) {
	node := sdb.db.From(nodeNameBlocks)
	var pair blockReferencePointIDPair
	if rp.IsBlockHeight() {
		rp++
	}
	err := node.One(nodeBlocksKeyReference, rp, &pair)
	if err != nil {
		return BlockFacts{}, err
	}
	return sdb.GetBlockFacts(pair.BlockID)
}

func (sdb *StormDB) GetTransaction(id types.TransactionID) (Transaction, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetTransaction(id)
}

func (sdb *StormDB) GetOutput(id types.OutputID) (Output, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetOutput(id)
}

func (sdb *StormDB) GetFreeForAllWallet(uh types.UnlockHash) (FreeForAllWalletData, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetFreeForAllWallet(uh)
}

func (sdb *StormDB) GetSingleSignatureWallet(uh types.UnlockHash) (SingleSignatureWalletData, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetSingleSignatureWallet(uh)
}

func (sdb *StormDB) GetMultiSignatureWallet(uh types.UnlockHash) (MultiSignatureWalletData, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetMultiSignatureWallet(uh)
}

func (sdb *StormDB) GetAtomicSwapContract(uh types.UnlockHash) (AtomicSwapContract, error) {
	node := newStormObjectNodeReader(sdb.db)
	return node.GetAtomicSwapContract(uh)
}

func (sdb *StormDB) GetPublicKey(uh types.UnlockHash) (types.PublicKey, error) {
	node := sdb.db.From(nodeNamePublicKeys)
	var pair unlockHashPublicKeyPair
	err := node.One(nodePublicKeysKeyUnlockHash, uh, &pair)
	if err != nil {
		return types.PublicKey{}, err
	}
	return pair.PublicKey, nil
}

func (sdb *StormDB) Close() error {
	return sdb.db.Close()
}
