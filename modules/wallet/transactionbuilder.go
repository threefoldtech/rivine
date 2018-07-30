package wallet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/rivine/rivine/build"
	"github.com/rivine/rivine/modules"
	"github.com/rivine/rivine/types"
)

var (
	// errBuilderAlreadySigned indicates that the transaction builder has
	// already added at least one successful signature to the transaction,
	// meaning that future calls to Sign will result in an invalid transaction.
	errBuilderAlreadySigned = errors.New("sign has already been called on this transaction builder, multiple calls can cause issues")
)

// transactionBuilder allows transactions to be manually constructed, including
// the ability to fund transactions with siacoins and blockstakes from the wallet.
type transactionBuilder struct {
	// 'signed' indicates that at least one transaction signature has been
	// added to the wallet, meaning that future calls to 'Sign' will fail.
	parents     []types.Transaction
	signed      bool
	transaction types.Transaction

	newParents       []int
	coinInputs       []inputSignContext
	blockstakeInputs []inputSignContext

	wallet *Wallet
}

type inputSignContext struct {
	InputIndex int
	UnlockHash types.UnlockHash
}

// FundCoins will add a coin input of exactly 'amount' to the
// transaction. The coin input will not be signed until 'Sign' is called
// on the transaction builder.
func (tb *transactionBuilder) FundCoins(amount types.Currency) error {
	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	consensusSetHeight, err := dbGetConsensusHeight(tb.wallet.dbTx)
	if err != nil {
		return err
	}

	if !tb.wallet.unlocked {
		return modules.ErrLockedWallet
	}

	// prepare fulfillable context
	ctx := tb.wallet.getFulfillableContextForLatestBlock()

	// Collect a value-sorted set of fulfillable coin outputs.
	var so sortedOutputs
	dbForEachCoinOutput(tb.wallet.dbTx, func(id types.CoinOutputID, co types.CoinOutput) {
		if !co.Condition.Fulfillable(ctx) {
			return
		}
		so.ids = append(so.ids, id)
		so.outputs = append(so.outputs, co)
	})
	// Add all of the unconfirmed outputs as well.
	for _, upt := range tb.wallet.unconfirmedProcessedTransactions {
		for i, sco := range upt.Transaction.CoinOutputs {
			uh := sco.Condition.UnlockHash()
			// Determine if the output belongs to the wallet.
			exists, err := tb.wallet.keyExists(uh)
			if err != nil {
				return err
			}
			if !exists || !sco.Condition.Fulfillable(ctx) {
				continue
			}
			so.ids = append(so.ids, upt.Transaction.CoinOutputID(uint64(i)))
			so.outputs = append(so.outputs, sco)
		}
	}
	sort.Sort(sort.Reverse(so))

	// Create a transaction that will add the correct amount of siacoins to the
	// transaction.
	var fund types.Currency
	// potentialFund tracks the balance of the wallet including outputs that
	// have been spent in other unconfirmed transactions recently. This is to
	// provide the user with a more useful error message in the event that they
	// are overspending.
	var potentialFund types.Currency
	var spentScoids []types.CoinOutputID
	for i := range so.ids {
		scoid := so.ids[i]
		sco := so.outputs[i]
		// Check that this output has not recently been spent by the wallet.
		spendHeight, err := dbGetSpentOutput(tb.wallet.dbTx, types.OutputID(scoid))
		if err != nil && err != errNoKey {
			return err
		}
		// Prevent an underflow error.
		allowedHeight := consensusSetHeight - RespendTimeout
		if consensusSetHeight < RespendTimeout {
			allowedHeight = 0
		}
		if spendHeight > allowedHeight {
			potentialFund = potentialFund.Add(sco.Value)
			continue
		}

		// prepare fulfillment, matching the output
		uh := sco.Condition.UnlockHash()
		var ff types.MarshalableUnlockFulfillment
		switch sco.Condition.ConditionType() {
		case types.ConditionTypeUnlockHash, types.ConditionTypeTimeLock:
			// ConditionTypeTimeLock is fine, as we know it's fulfillable,
			// and that can only mean for now that it is using an internal unlockHashCondition or nilCondition
			pk, _, err := tb.wallet.getKey(uh)
			if err != nil {
				return err
			}
			ff = types.NewSingleSignatureFulfillment(pk)
		default:
			if build.DEBUG {
				panic(fmt.Sprintf("unexpected condition type: %[1]v (%[1]T)", sco.Condition))
			}
			return types.ErrUnexpectedUnlockCondition
		}
		// Add a coin input for this output.
		sci := types.CoinInput{
			ParentID:    scoid,
			Fulfillment: types.NewFulfillment(ff),
		}
		tb.coinInputs = append(tb.coinInputs, inputSignContext{
			InputIndex: len(tb.transaction.CoinInputs),
			UnlockHash: uh,
		})
		tb.transaction.CoinInputs = append(tb.transaction.CoinInputs, sci)

		spentScoids = append(spentScoids, scoid)

		// Add the output to the total fund
		fund = fund.Add(sco.Value)
		potentialFund = potentialFund.Add(sco.Value)
		if fund.Cmp(amount) >= 0 {
			break
		}
	}
	if potentialFund.Cmp(amount) >= 0 && fund.Cmp(amount) < 0 {
		return modules.ErrIncompleteTransactions
	}
	if fund.Cmp(amount) < 0 {
		return modules.ErrLowBalance
	}

	// Create a refund output if needed.
	if !amount.Equals(fund) {
		refundUnlockHash, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
		if err != nil {
			return err
		}
		refundOutput := types.CoinOutput{
			Value:     fund.Sub(amount),
			Condition: types.NewCondition(types.NewUnlockHashCondition(refundUnlockHash)),
		}
		tb.transaction.CoinOutputs = append(tb.transaction.CoinOutputs, refundOutput)
	}

	// Mark all outputs that were spent as spent.
	for _, scoid := range spentScoids {
		dbPutSpentOutput(tb.wallet.dbTx, types.OutputID(scoid), consensusSetHeight)
	}
	return nil
}

// FundBlockStakes will add a blockstake input of exactly 'amount' to the
// transaction. The blockstake input will not be signed until 'Sign' is called
// on the transaction builder.
func (tb *transactionBuilder) FundBlockStakes(amount types.Currency) error {
	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	consensusSetHeight, err := dbGetConsensusHeight(tb.wallet.dbTx)
	if err != nil {
		return err
	}

	if !tb.wallet.unlocked {
		return modules.ErrLockedWallet
	}

	// prepare fulfillable context
	ctx := tb.wallet.getFulfillableContextForLatestBlock()

	// Create a transaction that will add the correct amount of siafunds to the
	// transaction.
	var fund types.Currency
	var potentialFund types.Currency
	var spentSfoids []types.BlockStakeOutputID

	var forEachErr error
	dbForEachBlockStakeOutput(tb.wallet.dbTx, func(id types.BlockStakeOutputID, bso types.BlockStakeOutput) {
		if !bso.Condition.Fulfillable(ctx) {
			return
		}
		// Check that this output has not recently been spent by the wallet.
		var spendHeight types.BlockHeight
		spendHeight, forEachErr = dbGetSpentOutput(tb.wallet.dbTx, types.OutputID(id))
		if forEachErr != nil {
			if forEachErr != errNoKey {
				return
			}
			forEachErr = nil
		}
		// Prevent an underflow error.
		allowedHeight := consensusSetHeight - RespendTimeout
		if consensusSetHeight < RespendTimeout {
			allowedHeight = 0
		}
		if spendHeight > allowedHeight {
			potentialFund = potentialFund.Add(bso.Value)
			return
		}

		// prepare fulfillment, matching the output
		uh := bso.Condition.UnlockHash()
		var ff types.MarshalableUnlockFulfillment
		switch bso.Condition.ConditionType() {
		case types.ConditionTypeUnlockHash, types.ConditionTypeTimeLock:
			// ConditionTypeTimeLock is fine, as we know it's fulfillable,
			// and that can only mean for now that it is using an internal unlockHashCondition or nilCondition
			var pk types.SiaPublicKey
			pk, _, forEachErr = tb.wallet.getKey(uh)
			if forEachErr != nil {
				return
			}
			ff = types.NewSingleSignatureFulfillment(pk)
		default:
			if build.DEBUG {
				panic(fmt.Sprintf("unexpected condition type: %[1]v (%[1]T)", bso.Condition))
			}
			forEachErr = types.ErrUnexpectedUnlockCondition
			return
		}
		// Add a block stake input for this output.
		sfi := types.BlockStakeInput{
			ParentID:    id,
			Fulfillment: types.NewFulfillment(ff),
		}
		tb.blockstakeInputs = append(tb.blockstakeInputs, inputSignContext{
			InputIndex: len(tb.transaction.BlockStakeInputs),
			UnlockHash: uh,
		})
		tb.transaction.BlockStakeInputs = append(tb.transaction.BlockStakeInputs, sfi)

		spentSfoids = append(spentSfoids, id)

		// Add the output to the total fund
		fund = fund.Add(bso.Value)
		potentialFund = potentialFund.Add(bso.Value)
		if fund.Cmp(amount) >= 0 {
			return
		}
	})
	if forEachErr != nil {
		return forEachErr
	}
	if potentialFund.Cmp(amount) >= 0 && fund.Cmp(amount) < 0 {
		return modules.ErrIncompleteTransactions
	}
	if fund.Cmp(amount) < 0 {
		return modules.ErrLowBalance
	}

	// Create a refund output if needed.
	if !amount.Equals(fund) {
		refundUnlockHash, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
		if err != nil {
			return err
		}
		refundOutput := types.BlockStakeOutput{
			Value:     fund.Sub(amount),
			Condition: types.NewCondition(types.NewUnlockHashCondition(refundUnlockHash)),
		}
		tb.transaction.BlockStakeOutputs = append(tb.transaction.BlockStakeOutputs, refundOutput)
	}

	// Mark all outputs that were spent as spent.
	for _, sfoid := range spentSfoids {
		dbPutSpentOutput(tb.wallet.dbTx, types.OutputID(sfoid), consensusSetHeight)
	}
	return nil
}

// AddParents adds a set of parents to the transaction.
func (tb *transactionBuilder) AddParents(newParents []types.Transaction) {
	tb.parents = append(tb.parents, newParents...)
}

// AddMinerFee adds a miner fee to the transaction, returning the index of the
// miner fee within the transaction.
func (tb *transactionBuilder) AddMinerFee(fee types.Currency) uint64 {
	tb.transaction.MinerFees = append(tb.transaction.MinerFees, fee)
	return uint64(len(tb.transaction.MinerFees) - 1)
}

// AddCoinInput adds a siacoin input to the transaction, returning the index
// of the coin input within the transaction. When 'Sign' gets called, this
// input will be left unsigned.
func (tb *transactionBuilder) AddCoinInput(input types.CoinInput) uint64 {
	tb.transaction.CoinInputs = append(tb.transaction.CoinInputs, input)
	return uint64(len(tb.transaction.CoinInputs) - 1)
}

// AddCoinOutput adds a siacoin output to the transaction, returning the
// index of the siacoin output within the transaction.
func (tb *transactionBuilder) AddCoinOutput(output types.CoinOutput) uint64 {
	tb.transaction.CoinOutputs = append(tb.transaction.CoinOutputs, output)
	return uint64(len(tb.transaction.CoinOutputs) - 1)
}

// AddBlockStakeInput adds a blockstake input to the transaction, returning the index
// of the blockstake input within the transaction. When 'Sign' is called, this
// input will be left unsigned.
func (tb *transactionBuilder) AddBlockStakeInput(input types.BlockStakeInput) uint64 {
	tb.transaction.BlockStakeInputs = append(tb.transaction.BlockStakeInputs, input)
	return uint64(len(tb.transaction.BlockStakeInputs) - 1)
}

// SpendBlockStake will link the unspent block stake to the transaction as an input.
// In contrast with FundBlockStakes, this function will not loop over all unspent
// block stake output. the ubsoid is an argument. The blockstake input will not be
// signed until 'Sign' is called on the transaction builder.
func (tb *transactionBuilder) SpendBlockStake(ubsoid types.BlockStakeOutputID) error {
	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	consensusSetHeight, err := dbGetConsensusHeight(tb.wallet.dbTx)
	if err != nil {
		return err
	}

	if !tb.wallet.unlocked {
		return modules.ErrLockedWallet
	}

	ubso, err := dbGetBlockStakeOutput(tb.wallet.dbTx, ubsoid)
	if err != nil {
		return modules.ErrIncompleteTransactions //TODO: not right error
	}

	uh := ubso.Condition.UnlockHash()
	pk, _, err := tb.wallet.getKey(uh)
	if err != nil {
		return err
	}
	bsi := types.BlockStakeInput{
		ParentID:    ubsoid,
		Fulfillment: types.NewFulfillment(types.NewSingleSignatureFulfillment(pk)),
	}
	tb.blockstakeInputs = append(tb.blockstakeInputs, inputSignContext{
		InputIndex: len(tb.transaction.BlockStakeInputs),
		UnlockHash: uh,
	})
	tb.transaction.BlockStakeInputs = append(tb.transaction.BlockStakeInputs, bsi)

	// Mark output as spent.
	dbPutSpentOutput(tb.wallet.dbTx, types.OutputID(ubsoid), consensusSetHeight)
	return nil
}

// AddBlockStakeOutput adds a blockstake output to the transaction, returning the
// index of the blockstake output within the transaction.
func (tb *transactionBuilder) AddBlockStakeOutput(output types.BlockStakeOutput) uint64 {
	tb.transaction.BlockStakeOutputs = append(tb.transaction.BlockStakeOutputs, output)
	return uint64(len(tb.transaction.BlockStakeOutputs) - 1)
}

// AddArbitraryData sets the arbitrary data of the transaction.
func (tb *transactionBuilder) SetArbitraryData(arb []byte) {
	tb.transaction.ArbitraryData = arb
}

// Drop discards all of the outputs in a transaction, returning them to the
// pool so that other transactions may use them. 'Drop' should only be called
// if a transaction is both unsigned and will not be used any further.
func (tb *transactionBuilder) Drop() {
	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	// Iterate through all parents and the transaction itself and restore all
	// outputs to the list of available outputs.
	txns := append(tb.parents, tb.transaction)
	for _, txn := range txns {
		for _, sci := range txn.CoinInputs {
			dbDeleteSpentOutput(tb.wallet.dbTx, types.OutputID(sci.ParentID))
		}
	}

	tb.parents = nil
	tb.signed = false
	tb.transaction = types.Transaction{
		Version: tb.wallet.chainCts.DefaultTransactionVersion,
	}

	tb.newParents = nil
	tb.coinInputs = nil
	tb.blockstakeInputs = nil
}

// Sign will sign any inputs added by 'FundSiacoins' or 'FundSiafunds' and
// return a transaction set that contains all parents prepended to the
// transaction. If more fields need to be added, a new transaction builder will
// need to be created.
//
// If the whole transaction flag is set to true, then the whole transaction
// flag will be set in the covered fields object. If the whole transaction flag
// is set to false, then the covered fields object will cover all fields that
// have already been added to the transaction, but will also leave room for
// more fields to be added.
//
// Sign should not be called more than once. If, for some reason, there is an
// error while calling Sign, the builder should be dropped.
func (tb *transactionBuilder) Sign() ([]types.Transaction, error) {
	if tb.signed {
		return nil, errBuilderAlreadySigned
	}

	// For each siacoin input in the transaction that we added, provide a
	// signature.
	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	if !tb.wallet.unlocked {
		return nil, modules.ErrLockedWallet
	}

	for _, ctx := range tb.coinInputs {
		input := tb.transaction.CoinInputs[ctx.InputIndex]
		_, sk, err := tb.wallet.getKey(ctx.UnlockHash)
		if err != nil {
			return nil, err
		}
		err = input.Fulfillment.Sign(types.FulfillmentSignContext{
			InputIndex:  uint64(ctx.InputIndex),
			Transaction: tb.transaction,
			Key:         sk,
		})
		if err != nil {
			return nil, err
		}
		tb.signed = true // Signed is set to true after one successful signature to indicate that future signings can cause issues.
	}
	for _, ctx := range tb.blockstakeInputs {
		input := tb.transaction.BlockStakeInputs[ctx.InputIndex]
		_, sk, err := tb.wallet.getKey(ctx.UnlockHash)
		if err != nil {
			return nil, err
		}
		err = input.Fulfillment.Sign(types.FulfillmentSignContext{
			InputIndex:  uint64(ctx.InputIndex),
			Transaction: tb.transaction,
			Key:         sk,
		})
		if err != nil {
			return nil, err
		}
		tb.signed = true // Signed is set to true after one successful signature to indicate that future signings can cause issues.
	}

	// Get the transaction set and delete the transaction from the registry.
	txnSet := append(tb.parents, tb.transaction)
	return txnSet, nil
}

// AttemptSigning tries to sign any input for which keys are loaded in the wallet
func (tb *transactionBuilder) SignAllPossibleInputs() ([]types.Transaction, error) {
	if tb.signed {
		return nil, errBuilderAlreadySigned
	}

	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	if !tb.wallet.unlocked {
		return nil, modules.ErrLockedWallet
	}

	for i := range tb.transaction.CoinInputs {
		ci := &tb.transaction.CoinInputs[i]
		uco, err := tb.wallet.cs.GetCoinOutput(ci.ParentID)
		if err != nil {
			return nil, err
		}

		if err = tb.signCoinInput(i, ci, uco.Condition.Condition); err != nil {
			return nil, err
		}
		tb.signed = true

	}

	for i := range tb.transaction.BlockStakeInputs {
		bsi := &tb.transaction.BlockStakeInputs[i]
		ubso, err := tb.wallet.cs.GetBlockStakeOutput(bsi.ParentID)
		if err != nil {
			return nil, err
		}
		if err = tb.signBlockStakeInput(i, bsi, ubso.Condition.Condition); err != nil {
			return nil, err
		}
		tb.signed = true
	}

	// Get the transaction set and delete the transaction from the registry.
	txnSet := append(tb.parents, tb.transaction)
	return txnSet, nil
}

// signCoinInput attempts to sign a coin input with a key from the wallet
func (tb *transactionBuilder) signCoinInput(idx int, ci *types.CoinInput, cond types.MarshalableUnlockCondition) error {
	return tb.signFulfillment(idx, &ci.Fulfillment, cond)
}

// signBlockStakeInput attempts to sign a blockstake input with a key from the wallet
func (tb *transactionBuilder) signBlockStakeInput(idx int, bsi *types.BlockStakeInput, cond types.MarshalableUnlockCondition) error {
	return tb.signFulfillment(idx, &bsi.Fulfillment, cond)
}

func (tb *transactionBuilder) signFulfillment(idx int, fulfillment *types.UnlockFulfillmentProxy, cond types.MarshalableUnlockCondition) error {
	var err error
	switch uh := cond.UnlockHash(); uh.Type {
	case types.UnlockTypeNil:
		// Try to get a new (random) key to sign
		// we use nextPrimarySeedAddress, instead of NextAddres,
		// as the parent function of signFulfillment, SignAllPossibleInputs,
		// has already locked the wallet's mutex
		uh, err = tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
		if err != nil {
			return err
		}
		fallthrough
	case types.UnlockTypePubKey:
		if key, exists := tb.wallet.keys[uh]; exists {
			fulfillment.Fulfillment = types.NewSingleSignatureFulfillment(types.Ed25519PublicKey(key.PublicKey))
			err := fulfillment.Fulfillment.Sign(types.FulfillmentSignContext{
				InputIndex:  uint64(idx),
				Transaction: tb.transaction,
				Key:         key.SecretKey,
			})
			if err != nil {
				return err
			}
			tb.signed = true
		}

	case types.UnlockTypeMultiSig:
		uhs, _ := getMultisigConditionProperties(cond)
		if len(uhs) == 0 {
			return fmt.Errorf("unexpected condition type %T for multi sig condition", cond)
		}
		if fulfillment.FulfillmentType() == types.FulfillmentTypeNil {
			fulfillment.Fulfillment = &types.MultiSignatureFulfillment{}
		}
		for _, uh := range uhs {
			if key, exists := tb.wallet.keys[uh]; exists {
				err := fulfillment.Sign(types.FulfillmentSignContext{
					InputIndex:  uint64(idx),
					Transaction: tb.transaction,
					Key: types.KeyPair{
						PublicKey:  types.Ed25519PublicKey(key.PublicKey),
						PrivateKey: types.ByteSlice(key.SecretKey[:]),
					},
				})
				if err != nil {
					return err
				}
				tb.signed = true
			}
		}

	default:
		return fmt.Errorf("failed to sign fulfillment: unexpected condition type %T", cond)
	}

	return nil
}

// ViewTransaction returns a transaction-in-progress along with all of its
// parents, specified by id. An error is returned if the id is invalid.  Note
// that ids become invalid for a transaction after 'SignTransaction' has been
// called because the transaction gets deleted.
func (tb *transactionBuilder) View() (types.Transaction, []types.Transaction) {
	return tb.transaction, tb.parents
}

// ViewAdded returns all of the siacoin inputs, siafund inputs, and parent
// transactions that have been automatically added by the builder.
func (tb *transactionBuilder) ViewAdded() (newParents, coinInputs, blockstakeInputs []int) {
	newParents = tb.newParents
	for _, ci := range tb.coinInputs {
		coinInputs = append(coinInputs, ci.InputIndex)
	}
	for _, bsi := range tb.blockstakeInputs {
		blockstakeInputs = append(blockstakeInputs, bsi.InputIndex)
	}
	return
}

// RegisterTransaction takes a transaction and its parents and returns a
// transactionBuilder which can be used to expand the transaction. The most
// typical call is 'RegisterTransaction(types.Transaction{}, nil)', which
// registers a new transaction without parents.
func (w *Wallet) RegisterTransaction(t types.Transaction, parents []types.Transaction) modules.TransactionBuilder {
	// Create a deep copy of the transaction and parents by encoding them. A
	// deep copy ensures that there are no pointer or slice related errors -
	// the builder will be working directly on the transaction, and the
	// transaction may be in use elsewhere (in this case, the host is using the
	// transaction.
	pBytes := bytes.NewBuffer(nil)
	err := json.NewEncoder(pBytes).Encode(parents)
	if build.DEBUG && err != nil {
		panic("Failed to encode parent transactions: " + err.Error())
	}
	var pCopy []types.Transaction
	err = json.NewDecoder(pBytes).Decode(&pCopy)
	if build.DEBUG && err != nil {
		panic("Failed to decode parent transactions: " + err.Error())
	}
	tbytes := bytes.NewBuffer(nil)
	err = json.NewEncoder(tbytes).Encode(t)
	if build.DEBUG && err != nil {
		panic("Failed to encode transaction: " + err.Error())
	}
	var tCopy types.Transaction
	err = json.NewDecoder(tbytes).Decode(&tCopy)
	if build.DEBUG && err != nil {
		panic("Failed to decode transaction: " + err.Error())
	}
	return &transactionBuilder{
		parents:     pCopy,
		transaction: tCopy,
		wallet:      w,
	}
}

// StartTransaction is a convenience function that calls
// StartTransactionWithVersion with the DefaultTransactionVersion constant.
func (w *Wallet) StartTransaction() modules.TransactionBuilder {
	return w.StartTransactionWithVersion(w.chainCts.DefaultTransactionVersion)
}

// StartTransactionWithVersion is a convenience function that calls
// RegisterTransaction(types.Transaction{Version: version}, nil).
func (w *Wallet) StartTransactionWithVersion(version types.TransactionVersion) modules.TransactionBuilder {
	return w.RegisterTransaction(types.Transaction{
		Version: version,
	}, nil)
}
