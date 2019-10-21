// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package explorergraphql

import (
	"fmt"
	"io"
	"strconv"

	"github.com/threefoldtech/rivine/crypto"
	"github.com/threefoldtech/rivine/types"
)

type Contract interface {
	IsContract()
}

type Object interface {
	IsObject()
}

type Transaction interface {
	IsTransaction()
}

type UnlockCondition interface {
	IsUnlockCondition()
}

type UnlockFulfillment interface {
	IsUnlockFulfillment()
}

type Wallet interface {
	IsWallet()
}

type AtomicSwapCondition struct {
	Version      ByteVersion            `json:"Version"`
	UnlockHash   types.UnlockHash       `json:"UnlockHash"`
	Sender       *AtomicSwapParticipant `json:"Sender"`
	Receiver     *AtomicSwapParticipant `json:"Receiver"`
	HashedSecret BinaryData             `json:"HashedSecret"`
	TimeLock     LockTime               `json:"TimeLock"`
}

func (AtomicSwapCondition) IsUnlockCondition() {}

type AtomicSwapContract struct {
	UnlockHash          types.UnlockHash       `json:"UnlockHash"`
	ContractCondition   *AtomicSwapCondition   `json:"ContractCondition"`
	ContractFulfillment *AtomicSwapFulfillment `json:"ContractFulfillment"`
	ContractValue       BigInt                 `json:"ContractValue"`
	BlockHeight         types.BlockHeight      `json:"BlockHeight"`
	BlockTime           types.Timestamp        `json:"BlockTime"`
	Transactions        []Transaction          `json:"Transactions"`
	CoinInputs          []*Input               `json:"CoinInputs"`
	CoinOutputs         []*Output              `json:"CoinOutputs"`
}

func (AtomicSwapContract) IsObject()   {}
func (AtomicSwapContract) IsContract() {}

type AtomicSwapFulfillment struct {
	Version         ByteVersion      `json:"Version"`
	ParentCondition UnlockCondition  `json:"ParentCondition"`
	PublicKey       crypto.PublicKey `json:"PublicKey"`
	Signature       crypto.Signature `json:"Signature"`
	Secret          *BinaryData      `json:"Secret"`
}

func (AtomicSwapFulfillment) IsUnlockFulfillment() {}

type AtomicSwapParticipant struct {
	UnlockHash types.UnlockHash  `json:"UnlockHash"`
	PublicKey  *crypto.PublicKey `json:"PublicKey"`
}

type Balance struct {
	Unlocked BigInt `json:"Unlocked"`
	Locked   BigInt `json:"Locked"`
}

type Block struct {
	Header       *BlockHeader  `json:"Header"`
	Transactions []Transaction `json:"Transactions"`
}

func (Block) IsObject() {}

type BlockHeader struct {
	ID          crypto.Hash        `json:"ID"`
	BlockTime   *types.Timestamp   `json:"BlockTime"`
	BlockHeight *types.BlockHeight `json:"BlockHeight"`
	Payouts     []*BlockPayout     `json:"Payouts"`
}

type BlockPayout struct {
	Output *Output          `json:"Output"`
	Type   *BlockPayoutType `json:"Type"`
}

type Input struct {
	ID          crypto.Hash       `json:"ID"`
	Value       BigInt            `json:"Value"`
	Fulfillment UnlockFulfillment `json:"Fulfillment"`
	Parent      *Output           `json:"Parent"`
}

func (Input) IsObject() {}

type LockTimeCondition struct {
	Version    ByteVersion       `json:"Version"`
	UnlockHash *types.UnlockHash `json:"UnlockHash"`
	LockValue  LockTime          `json:"LockValue"`
	LockType   LockType          `json:"LockType"`
	Condition  UnlockCondition   `json:"Condition"`
}

func (LockTimeCondition) IsUnlockCondition() {}

type MintCoinCreationTransaction struct {
	ID               crypto.Hash             `json:"ID"`
	Version          ByteVersion             `json:"Version"`
	BlockID          crypto.Hash             `json:"BlockID"`
	BlockHeight      *types.BlockHeight      `json:"BlockHeight"`
	BlockTimestamp   *types.Timestamp        `json:"BlockTimestamp"`
	TransactionOrder *int                    `json:"TransactionOrder"`
	Nonce            BinaryData              `json:"Nonce"`
	MintCondition    UnlockCondition         `json:"MintCondition"`
	MintFulfillment  UnlockFulfillment       `json:"MintFulfillment"`
	CoinInputs       []*Input                `json:"CoinInputs"`
	CoinOutputs      []*Output               `json:"CoinOutputs"`
	FeePayouts       []*TransactionFeePayout `json:"FeePayouts"`
	ArbitraryData    *BinaryData             `json:"ArbitraryData"`
}

func (MintCoinCreationTransaction) IsObject()      {}
func (MintCoinCreationTransaction) IsTransaction() {}

type MintCoinDestructionTransaction struct {
	ID               crypto.Hash             `json:"ID"`
	Version          ByteVersion             `json:"Version"`
	BlockID          crypto.Hash             `json:"BlockID"`
	BlockHeight      *types.BlockHeight      `json:"BlockHeight"`
	BlockTimestamp   *types.Timestamp        `json:"BlockTimestamp"`
	TransactionOrder *int                    `json:"TransactionOrder"`
	CoinInputs       []*Input                `json:"CoinInputs"`
	CoinOutputs      []*Output               `json:"CoinOutputs"`
	FeePayouts       []*TransactionFeePayout `json:"FeePayouts"`
	ArbitraryData    *BinaryData             `json:"ArbitraryData"`
}

func (MintCoinDestructionTransaction) IsObject()      {}
func (MintCoinDestructionTransaction) IsTransaction() {}

type MintConditionDefinitionTransaction struct {
	ID               crypto.Hash             `json:"ID"`
	Version          ByteVersion             `json:"Version"`
	BlockID          crypto.Hash             `json:"BlockID"`
	BlockHeight      *types.BlockHeight      `json:"BlockHeight"`
	BlockTimestamp   *types.Timestamp        `json:"BlockTimestamp"`
	TransactionOrder *int                    `json:"TransactionOrder"`
	Nonce            BinaryData              `json:"Nonce"`
	MintCondition    UnlockCondition         `json:"MintCondition"`
	MintFulfillment  UnlockFulfillment       `json:"MintFulfillment"`
	NewMintCondition UnlockCondition         `json:"NewMintCondition"`
	CoinInputs       []*Input                `json:"CoinInputs"`
	CoinOutputs      []*Output               `json:"CoinOutputs"`
	FeePayouts       []*TransactionFeePayout `json:"FeePayouts"`
	ArbitraryData    *BinaryData             `json:"ArbitraryData"`
}

func (MintConditionDefinitionTransaction) IsObject()      {}
func (MintConditionDefinitionTransaction) IsTransaction() {}

type MultiSignatureCondition struct {
	Version                ByteVersion         `json:"Version"`
	UnlockHash             types.UnlockHash    `json:"UnlockHash"`
	UnlockHashes           []*types.UnlockHash `json:"UnlockHashes"`
	RequiredSignatureCount int                 `json:"RequiredSignatureCount"`
}

func (MultiSignatureCondition) IsUnlockCondition() {}

type MultiSignatureFulfillment struct {
	Version         ByteVersion        `json:"Version"`
	ParentCondition UnlockCondition    `json:"ParentCondition"`
	PublicKeys      []crypto.PublicKey `json:"PublicKeys"`
	Signatures      []crypto.Signature `json:"Signatures"`
}

func (MultiSignatureFulfillment) IsUnlockFulfillment() {}

type MultiSignatureWallet struct {
	UnlockHash             types.UnlockHash             `json:"UnlockHash"`
	Owners                 []*MultiSignatureWalletOwner `json:"Owners"`
	RequiredSignatureCount *int                         `json:"RequiredSignatureCount"`
	BlockHeight            types.BlockHeight            `json:"BlockHeight"`
	BlockTime              types.Timestamp              `json:"BlockTime"`
	Transactions           []Transaction                `json:"Transactions"`
	CoinInputs             []*Input                     `json:"CoinInputs"`
	CoinOutputs            []*Output                    `json:"CoinOutputs"`
	BlockStakeInputs       []*Input                     `json:"BlockStakeInputs"`
	BlockStakeOutputs      []*Output                    `json:"BlockStakeOutputs"`
	CoinBalance            *Balance                     `json:"CoinBalance"`
	BlockStakeBalance      *Balance                     `json:"BlockStakeBalance"`
}

func (MultiSignatureWallet) IsObject() {}
func (MultiSignatureWallet) IsWallet() {}

type MultiSignatureWalletOwner struct {
	UnlockHash types.UnlockHash  `json:"UnlockHash"`
	PublicKey  *crypto.PublicKey `json:"PublicKey"`
}

type NilCondition struct {
	Version    ByteVersion      `json:"Version"`
	UnlockHash types.UnlockHash `json:"UnlockHash"`
}

func (NilCondition) IsUnlockCondition() {}

type Output struct {
	ID        crypto.Hash     `json:"ID"`
	Value     BigInt          `json:"Value"`
	Condition UnlockCondition `json:"Condition"`
	Sibling   *Input          `json:"Sibling"`
}

func (Output) IsObject() {}

type SingleSignatureFulfillment struct {
	Version         ByteVersion      `json:"Version"`
	ParentCondition UnlockCondition  `json:"ParentCondition"`
	PublicKey       crypto.PublicKey `json:"PublicKey"`
	Signature       crypto.Signature `json:"Signature"`
}

func (SingleSignatureFulfillment) IsUnlockFulfillment() {}

type SingleSignatureWallet struct {
	UnlockHash            types.UnlockHash        `json:"UnlockHash"`
	PublicKey             *crypto.PublicKey       `json:"PublicKey"`
	MultiSignatureWallets []*MultiSignatureWallet `json:"MultiSignatureWallets"`
	BlockHeight           types.BlockHeight       `json:"BlockHeight"`
	BlockTime             types.Timestamp         `json:"BlockTime"`
	Transactions          []Transaction           `json:"Transactions"`
	CoinInputs            []*Input                `json:"CoinInputs"`
	CoinOutputs           []*Output               `json:"CoinOutputs"`
	BlockStakeInputs      []*Input                `json:"BlockStakeInputs"`
	BlockStakeOutputs     []*Output               `json:"BlockStakeOutputs"`
	CoinBalance           *Balance                `json:"CoinBalance"`
	BlockStakeBalance     *Balance                `json:"BlockStakeBalance"`
}

func (SingleSignatureWallet) IsObject() {}
func (SingleSignatureWallet) IsWallet() {}

type StandardTransaction struct {
	ID                crypto.Hash             `json:"ID"`
	Version           ByteVersion             `json:"Version"`
	BlockID           crypto.Hash             `json:"BlockID"`
	BlockHeight       *types.BlockHeight      `json:"BlockHeight"`
	BlockTimestamp    *types.Timestamp        `json:"BlockTimestamp"`
	TransactionOrder  *int                    `json:"TransactionOrder"`
	CoinInputs        []*Input                `json:"CoinInputs"`
	CoinOutputs       []*Output               `json:"CoinOutputs"`
	BlockStakeInputs  []*Input                `json:"BlockStakeInputs"`
	BlockStakeOutputs []*Output               `json:"BlockStakeOutputs"`
	FeePayouts        []*TransactionFeePayout `json:"FeePayouts"`
	ArbitraryData     *BinaryData             `json:"ArbitraryData"`
}

func (StandardTransaction) IsObject()      {}
func (StandardTransaction) IsTransaction() {}

type TransactionFeePayout struct {
	BlockPayout *BlockPayout `json:"BlockPayout"`
	Value       BigInt       `json:"Value"`
}

type UnlockHashCondition struct {
	Version    ByteVersion      `json:"Version"`
	UnlockHash types.UnlockHash `json:"UnlockHash"`
}

func (UnlockHashCondition) IsUnlockCondition() {}

type BlockPayoutType string

const (
	BlockPayoutTypeBlockReward    BlockPayoutType = "BLOCK_REWARD"
	BlockPayoutTypeTransactionFee BlockPayoutType = "TRANSACTION_FEE"
)

var AllBlockPayoutType = []BlockPayoutType{
	BlockPayoutTypeBlockReward,
	BlockPayoutTypeTransactionFee,
}

func (e BlockPayoutType) IsValid() bool {
	switch e {
	case BlockPayoutTypeBlockReward, BlockPayoutTypeTransactionFee:
		return true
	}
	return false
}

func (e BlockPayoutType) String() string {
	return string(e)
}

func (e *BlockPayoutType) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = BlockPayoutType(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid BlockPayoutType", str)
	}
	return nil
}

func (e BlockPayoutType) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type LockType string

const (
	LockTypeBlockHeight LockType = "BLOCK_HEIGHT"
	LockTypeTimestamp   LockType = "TIMESTAMP"
)

var AllLockType = []LockType{
	LockTypeBlockHeight,
	LockTypeTimestamp,
}

func (e LockType) IsValid() bool {
	switch e {
	case LockTypeBlockHeight, LockTypeTimestamp:
		return true
	}
	return false
}

func (e LockType) String() string {
	return string(e)
}

func (e *LockType) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = LockType(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid LockType", str)
	}
	return nil
}

func (e LockType) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
