package consensus

import (
	"path/filepath"

	"github.com/threefoldtech/rivine/build"
	"github.com/threefoldtech/rivine/crypto"
	"github.com/threefoldtech/rivine/modules"
	"github.com/threefoldtech/rivine/modules/gateway"
	"github.com/threefoldtech/rivine/types"
)

// A consensusSetTester is the helper object for consensus set testing,
// including helper modules and methods for controlling synchronization between
// the tester and the modules.
type consensusSetTester struct {
	gateway   modules.Gateway
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	cs *ConsensusSet

	persistDir string
}

/*TODO: enable and fix?
// randAddress returns a random address that is not spendable.
func randAddress() types.UnlockHash {
	var uh types.UnlockHash
	_, err := rand.Read(uh[:])
	if err != nil {
		panic(err)
	}
	return uh
}

//TODO: rename to blockstakes and fix
// addSiafunds makes a transaction that moves some testing genesis siafunds
// into the wallet.
func (cst *consensusSetTester) addSiafunds() {
	// Get an address to receive the siafunds.
	uc, err := cst.wallet.NextAddress()
	if err != nil {
		panic(err)
	}

	// Create the transaction that sends the anyone-can-spend siafund output to
	// the wallet address (output only available during testing).
	txn := types.Transaction{
		Version: cst.cs.chainCts.DefaultTransactionVersion,
		BlockStakeInputs: []types.BlockStakeInput{{
			ParentID:         cst.cs.blockRoot.Block.Transactions[0].BlockStakeOutputID(2),
			UnlockConditions: types.UnlockConditions{},
		}},
		BlockStakeOutputs: []types.BlockStakeOutput{{
			Value:      types.NewCurrency64(1e3),
			UnlockHash: uc.UnlockHash(),
		}},
	}

	// Mine the transaction into the blockchain.
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		panic(err)
	}
	// TODO: make sure it is added
	// _, err = cst.miner.AddBlock()
	// if err != nil {
	// 	panic(err)
	// }

	// Check that the blockstakes made it to the wallet.
	_, blockstakeBalance := cst.wallet.ConfirmedBalance()
	if !blockstakeBalance.Equals64(1e3) {
		panic("wallet does not have the blockstakes")
	}
}
*/
// blankConsensusSetTester creates a consensusSetTester that has only the
// genesis block.
func blankConsensusSetTester(name string) (*consensusSetTester, error) {
	testdir := build.TempDir(modules.ConsensusDir, name)

	// Create modules.
	g, err := gateway.New("localhost:0", false, 1, filepath.Join(testdir, modules.GatewayDir), types.DefaultBlockchainInfo(), types.TestnetChainConstants(), nil, false)
	if err != nil {
		return nil, err
	}
	cs, err := New(g, false, filepath.Join(testdir, modules.ConsensusDir), types.DefaultBlockchainInfo(), types.TestnetChainConstants(), false)
	if err != nil {
		return nil, err
	}

	// Assemble all objects into a consensusSetTester.
	cst := &consensusSetTester{
		gateway: g,

		cs: cs,

		persistDir: testdir,
	}
	return cst, nil
}

/*
// createConsensusSetTester creates a consensusSetTester that's ready for use,
// including siacoins and siafunds available in the wallet.
func createConsensusSetTester(name string) (*consensusSetTester, error) {
	cst, err := blankConsensusSetTester(name)
	if err != nil {
		return nil, err
	}
	cst.addSiafunds()
	//TODO: make sure it get's accepted in the chain
	//cst.mineSiacoins()
	return cst, nil
}

// Close safely closes the consensus set tester. Because there's not a good way
// to errcheck when deferring a close, a panic is called in the event of an
// error.
func (cst *consensusSetTester) Close() error {
	errs := []error{
		cst.cs.Close(),
		cst.gateway.Close(),
	}
	if err := build.JoinErrors(errs, "; "); err != nil {
		panic(err)
	}
	return nil
}

// TestNilInputs tries to create new consensus set modules using nil inputs.
func TestNilInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	_, err := New(nil, false, testdir)
	if err != errNilGateway {
		t.Fatal(err)
	}
}

// TestClosing tries to close a consenuss set.
func TestDatabaseClosing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	testdir := build.TempDir(modules.ConsensusDir, t.Name())

	// Create the gateway.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, false, testdir)
	if err != nil {
		t.Fatal(err)
	}
	err = cs.Close()
	if err != nil {
		t.Error(err)
	}
}
*/
