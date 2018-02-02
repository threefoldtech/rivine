package explorer

import (
	"testing"

	"github.com/rivine/rivine/types"
)

// TestImmediateBlockFacts grabs the block facts object from the block explorer
// at the current height and verifies that the data has been filled out.
func TestImmediateBlockFacts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	et, err := createExplorerTester("TestImmediateBlockFacts")
	if err != nil {
		t.Fatal(err)
	}

	facts := et.explorer.LatestBlockFacts()
	var explorerHeight types.BlockHeight
	err = et.explorer.db.View(dbGetInternal(internalBlockHeight, &explorerHeight))
	if err != nil {
		t.Fatal(err)
	}
	if facts.Height != explorerHeight || explorerHeight == 0 {
		t.Error("wrong height reported in facts object")
	}
	// TODO: CalculateNumSiacoins has been removed in https://github.com/rivine/rivine/commit/8675b2afff5f200fe6c7d3fca7c21811e65f446a#diff-fd289e47592d409909487becb9d38925
	if facts.TotalCoins.Cmp(types.CalculateNumCoins(et.cs.Height())) != 0 {
		t.Error("wrong number of total coins:", facts.TotalCoins, et.cs.Height())
	}
}

// TestBlock probes the Block function of the explorer.
func TestBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	et, err := createExplorerTester("TestBlock")
	if err != nil {
		t.Fatal(err)
	}

	gb := types.GenesisBlock
	gbFetch, height, exists := et.explorer.Block(gb.ID())
	if !exists || height != 0 || gbFetch.ID() != gb.ID() {
		t.Error("call to 'Block' inside explorer failed")
	}
}

// TestBlockFacts checks that the correct block facts are returned for a query.
func TestBlockFacts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	et, err := createExplorerTester("TestBlockFacts")
	if err != nil {
		t.Fatal(err)
	}

	gb := types.GenesisBlock
	bf, exists := et.explorer.BlockFacts(0)
	if !exists || bf.BlockID != gb.ID() || bf.Height != 0 {
		t.Error("call to 'BlockFacts' inside explorer failed")
		t.Error("Expecting true ->", exists)
		t.Error("Expecting", gb.ID(), "->", bf.BlockID)
		t.Error("Expecting 0 ->", bf.Height)
	}

	bf, exists = et.explorer.BlockFacts(1)
	if !exists || bf.Height != 1 {
		t.Error("call to 'BlockFacts' has failed")
	}
}
