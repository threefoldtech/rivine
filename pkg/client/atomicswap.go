package client

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rivine/rivine/api"
	"github.com/rivine/rivine/pkg/cli"
	"github.com/rivine/rivine/types"
	"github.com/spf13/cobra"
)

var (
	atomicSwapCmd = &cobra.Command{
		Use:   "atomicswap",
		Short: "Create and interact with atomic swap contracts.",
		Long:  "Create and audit atomic swap contracts, as well as redeem money from them.",
	}

	atomicSwapParticipateCmd = &cobra.Command{
		Use:   "participate <initiator address> <amount> <secret hash>",
		Short: "Create an atomic swap contract as participant.",
		Long: `Create an atomic swap contract as a participant,
using the secret hash given by the initiator.`,
		Run: Wrap(atomicswapparticipatecmd),
	}

	atomicSwapInitiateCmd = &cobra.Command{
		Use:   "initiate <participant address> <amount>",
		Short: "Create an atomic swap contract as initiator.",
		Long: `Create an atomic swap contract as an initiator,
such that you can share its public information with the participant.`,
		Run: Wrap(atomicswapinitiatecmd),
	}

	atomicSwapAuditCmd = &cobra.Command{
		Use:   "auditcontract outputid",
		Short: "Audit a created atomic swap contract.",
		Long: `Audit a created atomic swap contract.

It will try to look for the given outputid in the consensus as an unspent coin output.
Should it not be find as an unspent output it will try to look in the transaction pool
as to confirm it might be not part of a created block yet.

Optionally the participant's address, currency amount and secret hash can be validated,
by giving one, some or all of them as flag arguments.

Should an unspent atomic swap contract be found, it will be printed to the STDOUT formatted
in a human-optimized format.
`,
		Run: atomicswapauditcmd,
	}

	atomicSwapExtractSecretCmd = &cobra.Command{
		Use:   "extractsecret transactionid [outputid]",
		Short: "Extract the secret from a redeemed swap contract.",
		Long: `Extract the secret from a redeemed atomic swap contract.

It will look for a transaction in the consensus set, using the given transactionID.
The transaction should contain at least one atomic swap contract fulfillment.
If an outputID is given, the (coin) input, from which the secret is to be extracted,
has to have the given outputID as parent ID, otherwise the first input is used,
which has an atomic swap contract fulfillment.

If it was spend as a refund, than no secret can be extracted.

Optionally the expected hashedsecret can be given (as a flag), as to ensure that the
secret hash is as expected, and also matches the found/extracted secret.`,
		Run: atomicswapextractsecretcmd,
	}

	atomicSwapRedeemCmd = &cobra.Command{
		Use:   "redeem outputid secret",
		Short: "Redeem the coins locked in an atomic swap contract.",
		Long:  "Redeem the coins locked in an atomic swap contract intended for you.",
		Run:   Wrap(atomicswapredeemcmd),
	}

	atomicSwapRefundCmd = &cobra.Command{
		Use:   "refund outputid",
		Short: "Refund the coins locked in an atomic swap contract.",
		Long:  "Refund the coins locked in an atomic swap contract created by you.",
		Run:   Wrap(atomicswaprefundcmd),
	}
)

var (
	atomicSwapParticipatecfg struct {
		duration         time.Duration
		sourceUnlockHash types.UnlockHash
	}
	atomicSwapInitiatecfg struct {
		duration         time.Duration
		sourceUnlockHash types.UnlockHash
	}
	atomicSwapAuditcfg struct {
		ParticipatorAddress types.UnlockHash
		CoinAmount          coinFlag
		HashedSecret        types.AtomicSwapHashedSecret
		MinDurationLeft     time.Duration
	}
	atomicSwapExtractSecretcfg struct {
		HashedSecret types.AtomicSwapHashedSecret
	}
)

func atomicswapparticipatecmd(participantAddress, amount, hashedSecret string) {
	var (
		condition types.AtomicSwapCondition
	)

	hastings, err := _CurrencyConvertor.ParseCoinString(amount)
	if err != nil {
		fmt.Fprintln(os.Stderr, _CurrencyConvertor.CoinArgDescription("amount"))
		Die("Could not parse amount:", err)
	}
	if hastings.Cmp(_MinimumTransactionFee) != 1 {
		Die("Cannot create atomic swap contract! Contracts which lock a value less than or equal to miner fees are currently not supported!")
	}

	err = condition.Receiver.LoadString(participantAddress)
	if err != nil {
		Die("Could not parse participant address (unlock hash):", err)
	}

	if atomicSwapParticipatecfg.sourceUnlockHash.Type != 0 {
		// use the hash given by the user explicitly
		condition.Sender = atomicSwapParticipatecfg.sourceUnlockHash
	} else {
		// get new one from the wallet
		resp := new(api.WalletAddressGET)
		err := _DefaultClient.httpClient.GetAPI("/wallet/address", resp)
		if err != nil {
			Die("Could not generate new address:", err)
		}
		condition.Sender = resp.Address
	}

	if hsl := len(hashedSecret); hsl == types.AtomicSwapHashedSecretLen*2 {
		_, err := hex.Decode(condition.HashedSecret[:], []byte(hashedSecret))
		if err != nil {
			Die("Invalid secret hash:", err)
		}
	} else {
		Die("Invalid secret hash length")
	}
	if atomicSwapParticipatecfg.duration == 0 {
		Die("Duration is required")
	}
	condition.TimeLock = types.OffsetTimestamp(atomicSwapParticipatecfg.duration)

	// print contract for review
	printContractInfo(hastings, condition, types.AtomicSwapSecret{})
	fmt.Println("")

	// ensure user wants to continue with creating the contract as it is (aka publishing it)
	if !askYesNoQuestion("Publish atomic swap (participation) transaction?") {
		Die("Atomic swap participation cancelled!")
	}

	// publish contract
	body, err := json.Marshal(api.WalletTransactionPOST{
		Condition: types.NewCondition(&condition),
		Amount:    hastings,
	})
	if err != nil {
		Die("Couldn't create/marshal JSOn body:", err)
	}
	var response api.WalletTransactionPOSTResponse
	err = _DefaultClient.httpClient.PostResp("/wallet/transaction", string(body), &response)
	if err != nil {
		Die("Couldn't create transaction:", err)
	}

	// find coinOutput and return its ID if possible
	coinOutputIndex, unlockHash := -1, condition.UnlockHash()
	for idx, co := range response.Transaction.CoinOutputs {
		if unlockHash.Cmp(co.Condition.UnlockHash()) == 0 {
			coinOutputIndex = idx
			break
		}
	}
	if coinOutputIndex == -1 {
		panic("didn't find atomic swap contract registered in any returned coin output")
	}
	fmt.Println("published contract transaction")
	fmt.Println("OutputID:", response.Transaction.CoinOutputID(uint64(coinOutputIndex)))
	fmt.Println("TransactionID:", response.Transaction.ID())
}

func atomicswapinitiatecmd(participatorAddress, amount string) {
	var (
		condition types.AtomicSwapCondition
	)

	hastings, err := _CurrencyConvertor.ParseCoinString(amount)
	if err != nil {
		fmt.Fprintln(os.Stderr, _CurrencyConvertor.CoinArgDescription("amount"))
		Die("Could not parse amount:", err)
	}
	if hastings.Cmp(_MinimumTransactionFee) != 1 {
		Die("Cannot create atomic swap contract! Contracts which lock a value less than or equal to miner fees are currently not supported!")
	}

	err = condition.Receiver.LoadString(participatorAddress)
	if err != nil {
		Die("Could not parse participator address (unlock hash):", err)
	}

	if atomicSwapInitiatecfg.sourceUnlockHash.Type != 0 {
		// use the hash given by the user explicitly
		condition.Sender = atomicSwapInitiatecfg.sourceUnlockHash
	} else {
		// get new one from the wallet
		resp := new(api.WalletAddressGET)
		err := _DefaultClient.httpClient.GetAPI("/wallet/address", resp)
		if err != nil {
			Die("Could not generate new address:", err)
		}
		condition.Sender = resp.Address
	}

	secret, err := types.NewAtomicSwapSecret()
	if err != nil {
		Die("failed to crypto-generate secret:", err)
	}
	condition.HashedSecret = types.NewAtomicSwapHashedSecret(secret)

	if atomicSwapInitiatecfg.duration == 0 {
		Die("Duration is required")
	}
	condition.TimeLock = types.OffsetTimestamp(atomicSwapInitiatecfg.duration)

	// print contract for review
	printContractInfo(hastings, condition, secret)
	fmt.Println("")

	// ensure user wants to continue with creating the contract as it is (aka publishing it)
	if !askYesNoQuestion("Publish atomic swap (initiating) transaction?") {
		Die("Atomic swap initiating cancelled!")
	}

	// publish contract
	body, err := json.Marshal(api.WalletTransactionPOST{
		Condition: types.NewCondition(&condition),
		Amount:    hastings,
	})
	if err != nil {
		Die("Couldn't create/marshal JSOn body:", err)
	}
	var response api.WalletTransactionPOSTResponse
	err = _DefaultClient.httpClient.PostResp("/wallet/transaction", string(body), &response)
	if err != nil {
		Die("Couldn't create transaction:", err)
	}

	// find coinOutput and return its ID if possible
	coinOutputIndex, unlockHash := -1, condition.UnlockHash()
	for idx, co := range response.Transaction.CoinOutputs {
		if unlockHash.Cmp(co.Condition.UnlockHash()) == 0 {
			coinOutputIndex = idx
			break
		}
	}
	if coinOutputIndex == -1 {
		panic("didn't find atomic swap contract registered in any returned coin output")
	}
	fmt.Println("published contract transaction")
	fmt.Println("OutputID:", response.Transaction.CoinOutputID(uint64(coinOutputIndex)))
	fmt.Println("TransactionID:", response.Transaction.ID())
}

func atomicswapauditcmd(cmd *cobra.Command, args []string) {
	argn := len(args)
	if argn < 1 || argn > 2 {
		cmd.UsageFunc()(cmd)
		os.Exit(ExitCodeUsage)
	}

	var (
		outputID      types.CoinOutputID
		transactionID types.TransactionID
	)

	err := outputID.LoadString(args[0])
	if err != nil {
		Die("failed to parse required positional (coin) outputID argument:", err)
	}
	if argn == 2 {
		err = transactionID.LoadString(args[1])
		if err != nil {
			Die("failed to parse optional positional transactionID argument:", err)
		}
	}

	// get unspent output from consensus
	var unspentCoinOutputResp api.ConsensusGetUnspentCoinOutput
	err = _DefaultClient.httpClient.GetAPI("/consensus/unspent/coinoutputs/"+outputID.String(), &unspentCoinOutputResp)
	if err == nil {
		auditAtomicSwapContract(unspentCoinOutputResp.Output, true)
		return
	}
	if err != errStatusNotFound {
		Die("Unexpected error occured while getting (unspent) coin output from consensus:", err)
	}
	// output couldn't be found as an unspent coin output
	// therefore the last positive hope is if it wasn't yet part of the transaction pool
	var txnPoolGetResp api.TransactionPoolGET
	err = _DefaultClient.httpClient.GetAPI("/transactionpool/transactions", &txnPoolGetResp)
	if err != nil {
		Die("Contract couldn't be found as part of an unspent coin output, and getting unconfirmed transactions from the transactionpool failed:", err)
	}
	for _, txn := range txnPoolGetResp.Transactions {
		for idx, co := range txn.CoinOutputs {
			coid := txn.CoinOutputID(uint64(idx))
			if coid == outputID {
				auditAtomicSwapContract(co, false)
				return
			}
		}
	}
	// given that we could have just hit the unlucky window,
	// where the block might have been just created in between our 2 calls,
	// let's try to get the coin output one last time from the consensus
	// contract couldn't be found as either
	err = _DefaultClient.httpClient.GetAPI("/consensus/unspent/coinoutputs/"+outputID.String(), &unspentCoinOutputResp)
	if err == nil {
		auditAtomicSwapContract(unspentCoinOutputResp.Output, true)
		return
	}
	if err != errStatusNotFound {
		Die("Unexpected error occured while getting (unspent) coin output from consensus (a second try):", err)
	}
	fmt.Printf(`Failed to find atomic swap contract using outputid %s.
It couldn't be found as part of a confirmed and unspent coin output in the consensus set,
neither could it be found as an unconfirmed coin output in the transaction pool.

This might mean one of 2 things:

+ Most likely it means that the given outputID is invalid;
+ It might instead mean that the atomic swap contract was already refunded or redeemed,
  this can be confirmed by looking the outputID up in a local, remote or public explorer;
`, outputID)
	DieWithExitCode(ExitCodeNotFound, "No unspent coin output could be found for ID "+outputID.String())
}

func auditAtomicSwapContract(co types.CoinOutput, confirmed bool) {
	condition, ok := co.Condition.Condition.(*types.AtomicSwapCondition)
	if !ok {
		Die(fmt.Sprintf(
			"Received unexpected condition of type %T, while type *types.AtomicSwapCondition was expected in order to be able to audit",
			co.Condition.Condition))
	}
	durationLeft := time.Unix(int64(condition.TimeLock), 0).Sub(computeTimeNow())

	fmt.Printf(`Atomic Swap Contract (condition) found:

Contract value: %s

Participator's address: %s
Initiator's address: %s
Secret Hash: %s
TimeLock: %[5]d (%[5]s)
TimeLock reached in: %s

`, _CurrencyConvertor.ToCoinStringWithUnit(co.Value), condition.Receiver,
		condition.Sender, condition.HashedSecret, condition.TimeLock, durationLeft)

	var invalidContract bool
	if !atomicSwapAuditcfg.CoinAmount.Amount.IsZero() {
		// optionally validate coin amount
		if !atomicSwapAuditcfg.CoinAmount.Amount.Equals(co.Value) {
			invalidContract = true
			fmt.Println("Unspent out's value " +
				_CurrencyConvertor.ToCoinStringWithUnit(co.Value) +
				" does not match the expected value " +
				_CurrencyConvertor.ToCoinStringWithUnit(atomicSwapAuditcfg.CoinAmount.Amount) + "!")
		}
	}
	if atomicSwapAuditcfg.HashedSecret != (types.AtomicSwapHashedSecret{}) {
		// optionally validate hashed secret
		if atomicSwapAuditcfg.HashedSecret != condition.HashedSecret {
			invalidContract = true
			fmt.Println("Found contract's secret hash " +
				condition.HashedSecret.String() +
				" does not match the expected secret hash " +
				atomicSwapAuditcfg.HashedSecret.String() + "!")
		}
	}
	if atomicSwapAuditcfg.ParticipatorAddress != (types.UnlockHash{}) {
		// optionally validate participator's address (unlockhash)
		if atomicSwapAuditcfg.ParticipatorAddress.Cmp(condition.Receiver) != 0 {
			invalidContract = true
			fmt.Println("Found contract's participator's address " +
				condition.Receiver.String() +
				" does not match the expected participator's address " +
				atomicSwapAuditcfg.ParticipatorAddress.String() + "!")
		}
	}
	if atomicSwapAuditcfg.MinDurationLeft != 0 {
		// optionally validate participator's address (unlockhash)
		if durationLeft < atomicSwapAuditcfg.MinDurationLeft {
			invalidContract = true
			fmt.Println("Found contract's duration left " +
				durationLeft.String() +
				" is not sufficient, when compared the expected duration left of " +
				atomicSwapAuditcfg.MinDurationLeft.String() + "!")
		}
	}
	if invalidContract {
		Die("Found Atomic Swap Contract does not meet the given expectations!")
	}
	fmt.Println("Found Atomic Swap Contract is valid :)")
	if !confirmed {
		fmt.Println("NOTE that this contract is still in the transaction pool and thus UNCONFIRMED!!!")
	}
}

// extractsecret transactionid [outputid]
func atomicswapextractsecretcmd(cmd *cobra.Command, args []string) {
	argn := len(args)
	if argn < 1 || argn > 2 {
		cmd.UsageFunc()(cmd)
		os.Exit(ExitCodeUsage)
	}

	var (
		txnID         types.TransactionID
		outputID      types.CoinOutputID
		outputIDGiven bool
		secret        types.AtomicSwapSecret
	)
	err := txnID.LoadString(args[0])
	if err != nil {
		Die("Couldn't parse first argment as a transaction (long) ID:", err)
	}
	if argn == 2 {
		err = outputID.LoadString(args[1])
		if err != nil {
			Die("Couldn't parse optional second argment as a coin outputID:", err)
		}
		outputIDGiven = true
	}

	// get transaction from consensus
	var txnResp api.ConsensusGetTransaction
	err = _DefaultClient.httpClient.GetAPI("/consensus/transactions/"+txnID.String(), &txnResp)
	if err != nil {
		Die("failed to get transaction:", err, "; Long ID:", txnID)
	}

	// get the secret from any of the inputs within this transaction, if possible,
	// or from an input which doesn't just define the right fulfillment but also has the right parentID
	for _, ci := range txnResp.CoinInputs {
		if outputIDGiven && ci.ParentID != outputID {
			continue
		}
		if ft := ci.Fulfillment.FulfillmentType(); ft != types.FulfillmentTypeAtomicSwap {
			continue
		}
		getter, ok := ci.Fulfillment.Fulfillment.(atomicSwapSecretGetter)
		if !ok {
			Die(fmt.Sprintf(
				"Received unexpected fulfillment type of type %T", ci.Fulfillment.Fulfillment))
		}
		secret = getter.AtomicSwapSecret()
		break
	}
	if secret == (types.AtomicSwapSecret{}) {
		Die("Couldn't find a matching atomic swap contract fulfillment in transaction with LongID: ", txnID)
	}

	if atomicSwapExtractSecretcfg.HashedSecret != (types.AtomicSwapHashedSecret{}) {
		hs := types.NewAtomicSwapHashedSecret(secret)
		if hs != atomicSwapExtractSecretcfg.HashedSecret {
			Die(fmt.Sprintf("found secret %s does not match expected and given secret hash %s",
				secret, atomicSwapExtractSecretcfg.HashedSecret))
		}
	}

	fmt.Println("Atomic swap contract was redeemed by participator! Success! :)")
	fmt.Println("Extracted secret:", secret.String())
}

type atomicSwapSecretGetter interface {
	AtomicSwapSecret() types.AtomicSwapSecret
}

func auditAtomicSwapFindSpendCoinSecret(coinOutputID types.CoinOutputID, txns []api.ExplorerTransaction) types.AtomicSwapSecret {
	for _, txn := range txns {
		for _, ci := range txn.RawTransaction.CoinInputs {
			if bytes.Compare(coinOutputID[:], ci.ParentID[:]) == 0 {
				asf, ok := ci.Fulfillment.Fulfillment.(atomicSwapSecretGetter)
				if !ok {
					Die("Given output was spend as a coin input, but fulfillment was not of type atomic swap!")
				}
				return asf.AtomicSwapSecret()
			}
		}
	}
	Die("Output was spend as a coin input, but couldn't find the transaction! (BUG?!")
	return types.AtomicSwapSecret{}
}
func auditAtomicSwapFindSpendBlockStakeSecret(blockStakeOutputID types.BlockStakeOutputID, txns []api.ExplorerTransaction) types.AtomicSwapSecret {
	for _, txn := range txns {
		for _, bsi := range txn.RawTransaction.BlockStakeInputs {
			if bytes.Compare(blockStakeOutputID[:], bsi.ParentID[:]) == 0 {
				asf, ok := bsi.Fulfillment.Fulfillment.(atomicSwapSecretGetter)
				if !ok {
					Die("Given output was spend as a block stake input, but fulfillment was not of type atomic swap!")
				}
				return asf.AtomicSwapSecret()
			}
		}
	}
	Die("Output was spend as a block stake input, but couldn't find the transaction! (BUG?!")
	return types.AtomicSwapSecret{}
}

// redeem outputid secret
func atomicswapredeemcmd(outputIDStr, secretStr string) {
	var (
		err      error
		outputID types.CoinOutputID
		secret   types.AtomicSwapSecret
	)

	// parse pos args
	err = outputID.LoadString(outputIDStr)
	if err != nil {
		Die("failed to parse outputid-argument:", err)
	}
	err = secret.LoadString(secretStr)
	if err != nil {
		Die("failed to parse secret-argument:", err)
	}
	if secret == (types.AtomicSwapSecret{}) {
		Die("secret cannot be all-nil when redeeming an atomic swap contract")
	}

	spendAtomicSwapContract(outputID, secret)
}

// refund outputid
func atomicswaprefundcmd(outputIDStr string) {
	var (
		err      error
		outputID types.CoinOutputID
	)

	// parse pos arg
	err = outputID.LoadString(outputIDStr)
	if err != nil {
		Die("failed to parse outputid-argument:", err)
	}

	spendAtomicSwapContract(outputID, types.AtomicSwapSecret{})
}

func spendAtomicSwapContract(outputID types.CoinOutputID, secret types.AtomicSwapSecret) {
	var keyWord string // define keyword for communication purposes
	if secret == (types.AtomicSwapSecret{}) {
		keyWord = "refund"
	} else {
		keyWord = "redeem"
	}

	// get unspent output from consensus
	var unspentCoinOutputResp api.ConsensusGetUnspentCoinOutput
	err := _DefaultClient.httpClient.GetAPI("/consensus/unspent/coinoutputs/"+outputID.String(), &unspentCoinOutputResp)
	if err != nil {
		Die("Could not get unspent coinoutput from consensus:", err)
	}

	// step 2: get correct spendable key from wallet
	if ct := unspentCoinOutputResp.Output.Condition.ConditionType(); ct != types.ConditionTypeAtomicSwap {
		Die("Only atomic swap conditions are supported, while referenced output is of type: ", ct)
	}
	condition, ok := unspentCoinOutputResp.Output.Condition.Condition.(*types.AtomicSwapCondition)
	if !ok {
		Die(fmt.Sprintf(
			"Received unexpected condition of type %T, while type *types.AtomicSwapCondition was expected",
			unspentCoinOutputResp.Output.Condition.Condition))
	}
	pk, sk := getSpendableKey(condition.Receiver)
	// quickly validate if returned sk matches the known unlock hash (sanity check)
	uh := types.NewPubKeyUnlockHash(pk)
	if uh.Cmp(condition.Receiver) != 0 {
		Die("Unexpected wallet public key returned:", sk)
	}

	if unspentCoinOutputResp.Output.Value.Cmp(_MinimumTransactionFee) != 1 {
		Die("Cannot " + keyWord + " atomic swap contracts which lock a value less than or equal to miner fees are currently not supported!")
	}

	// step 3: confirm contract details with user, before continuing
	// print contract for review
	printContractInfo(unspentCoinOutputResp.Output.Value, *condition, secret)
	fmt.Println("")
	// ensure user wants to continue with redeeming the contract!
	if !askYesNoQuestion("Publish atomic swap " + keyWord + " transaction?") {
		Die("Atomic swap " + keyWord + " transaction cancelled!")
	}

	// step 4: create a transaction
	txn := types.Transaction{
		Version: _DefaultTransactionVersion,
		CoinInputs: []types.CoinInput{
			{
				ParentID: outputID,
				Fulfillment: types.NewFulfillment(&types.AtomicSwapFulfillment{
					PublicKey: pk,
					Secret:    secret,
				}),
			},
		},
		CoinOutputs: []types.CoinOutput{
			{
				Condition: types.NewCondition(types.NewUnlockHashCondition(uh)),
				Value:     unspentCoinOutputResp.Output.Value.Sub(_MinimumTransactionFee),
			},
		},
		MinerFees: []types.Currency{_MinimumTransactionFee},
	}

	// step 5: sign transaction's only input
	err = txn.CoinInputs[0].Fulfillment.Sign(types.FulfillmentSignContext{
		InputIndex:  0,
		Transaction: txn,
		Key:         sk,
	})
	if err != nil {
		Die("Cannot "+keyWord+" atomic swap's locked coins! Couldn't sign transaction:", err)
	}

	// step 6: submit transaction to transaction pool and celebrate if possible
	txnid, err := commitTxn(txn)
	if err != nil {
		Die("Failed to "+keyWord+" atomic swaps locked tokens, as transaction couldn't commit:", err)
	}

	fmt.Println("")
	fmt.Println("Published atomic swap " + keyWord + " transaction!")
	fmt.Println("Transaction ID:", txnid)
	fmt.Println(`>   NOTE that this does NOT mean for 100% you'll have the money!
> Due to potential forks, double spending, and any other possible issues your
> ` + keyWord + ` might be declined by the network. Please check the network
> (e.g. using a public explorer node or your own full node) to ensure
> your payment went through. If not, try to audit the contract (again).`)
}

// get public- and private key from wallet module
func getSpendableKey(unlockHash types.UnlockHash) (types.SiaPublicKey, types.ByteSlice) {
	resp := new(api.WalletKeyGet)
	err := _DefaultClient.httpClient.GetAPI("/wallet/key/"+unlockHash.String(), resp)
	if err != nil {
		Die("Could not get a matching wallet public/secret key pair for the given unlock hash:", err)
	}
	if isNilByteSlice(resp.PublicKey) {
		Die("Could not get a wallet public key pair for the given unlock hash")
	}
	if isNilByteSlice(resp.SecretKey) {
		Die("Received matching public key, but no secret key was returned, is your wallet unlocked?!")
	}
	return types.SiaPublicKey{
		Algorithm: resp.AlgorithmSpecifier,
		Key:       resp.PublicKey,
	}, resp.SecretKey
}

func isNilByteSlice(bs types.ByteSlice) bool {
	for _, b := range bs {
		if b != 0 {
			return false
		}
	}
	return true
}

// commitTxn sends a transaction to the used node's transaction pool
func commitTxn(txn types.Transaction) (types.TransactionID, error) {
	bodyBuff := bytes.NewBuffer(nil)
	err := json.NewEncoder(bodyBuff).Encode(&txn)
	if err != nil {
		return types.TransactionID{}, err
	}

	resp := new(api.TransactionPoolPOST)
	err = _DefaultClient.httpClient.PostResp("/transactionpool/transactions", bodyBuff.String(), resp)
	return resp.TransactionID, err
}

func printContractInfo(hastings types.Currency, condition types.AtomicSwapCondition, secret types.AtomicSwapSecret) {
	var amountStr string
	if !hastings.Equals(types.Currency{}) {
		amountStr = fmt.Sprintf(`
Contract value: %s`, _CurrencyConvertor.ToCoinStringWithUnit(hastings))
	}

	var secretStr string
	if secret != (types.AtomicSwapSecret{}) {
		secretStr = fmt.Sprintf(`
Secret: %s`, secret)
	}

	cuh := condition.UnlockHash()

	fmt.Printf(`Contract address: %s%s
Participator's address: %s
Initiator's address: %s

SecretHash: %s%s

TimeLock: %[7]d (%[7]s)
TimeLock reached in: %s
`, cuh, amountStr, condition.Receiver, condition.Sender, condition.HashedSecret,
		secretStr, condition.TimeLock,
		time.Unix(int64(condition.TimeLock), 0).Sub(time.Now()))
}

func askYesNoQuestion(str string) bool {
	fmt.Printf("%s [Y/N] ", str)
	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		Die("failed to scan response:", err)
	}
	response = strings.ToLower(response)
	if containsString(okayResponses, response) {
		return true
	}
	if containsString(nokayResponses, response) {
		return false
	}

	fmt.Println("Please answer using 'yes' or 'no'")
	return askYesNoQuestion(str)
}

// posString returns the first index of element in slice.
// If slice does not contain element, returns -1.
func posString(slice []string, element string) int {
	for index, elem := range slice {
		if elem == element {
			return index
		}
	}
	return -1
}

// containsString returns true iff slice contains element
func containsString(slice []string, element string) bool {
	return !(posString(slice, element) == -1)
}

var (
	okayResponses  = []string{"y", "ye", "yes"}
	nokayResponses = []string{"n", "no", "noo", "nope"}
)

var computeTimeNow = func() time.Time {
	return time.Now()
}

func init() {
	atomicSwapParticipateCmd.Flags().DurationVarP(
		&atomicSwapParticipatecfg.duration, "duration", "d",
		time.Hour*24, "the duration of the atomic swap contract, the amount of time the participator has to collect")
	atomicSwapParticipateCmd.Flags().Var(cli.StringLoaderFlag{StringLoader: &atomicSwapParticipatecfg.sourceUnlockHash}, "initiator",
		"optionally define a wallet address (unlockhash) that is to be used for refunding purposes, one will be generated for you if none is given")

	atomicSwapInitiateCmd.Flags().DurationVarP(
		&atomicSwapInitiatecfg.duration, "duration", "d",
		time.Hour*48, "the duration of the atomic swap contract, the amount of time the participator has to collect")
	atomicSwapInitiateCmd.Flags().Var(cli.StringLoaderFlag{StringLoader: &atomicSwapInitiatecfg.sourceUnlockHash}, "initiator",
		"optionally define a wallet address (unlockhash) that is to be used for refunding purposes, one will be generated for you if none is given")

	atomicSwapAuditCmd.Flags().Var(
		cli.StringLoaderFlag{StringLoader: &atomicSwapAuditcfg.HashedSecret}, "secrethash",
		"optionally validate the expected secret hash to the secret hash of the found atomic swap contract condition")
	atomicSwapAuditCmd.Flags().Var(
		cli.StringLoaderFlag{StringLoader: &atomicSwapAuditcfg.ParticipatorAddress}, "participator",
		"optionally validate the given participator's address (unlockhash) to the one found in the atomic swap contract condition")
	atomicSwapAuditCmd.Flags().Var(
		&atomicSwapAuditcfg.CoinAmount, "amount",
		"optionally validate the given coin amount to the one found in the unspent coin output")
	atomicSwapAuditCmd.Flags().DurationVar(
		&atomicSwapAuditcfg.MinDurationLeft, "min-duration", 0,
		"optionally validate the given contract has sufficient duration left, as defined by the timelock in the found atomic swap contract condition")

	atomicSwapExtractSecretCmd.Flags().Var(
		cli.StringLoaderFlag{StringLoader: &atomicSwapExtractSecretcfg.HashedSecret}, "secrethash",
		"optionally validate the given secret hash with the secret hash of the found atomic swap contract condition")
}
