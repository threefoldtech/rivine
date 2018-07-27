package wallet

import (
	"github.com/rivine/rivine/build"
)

var (
	// lookaheadBuffer together with lookaheadRescanThreshold defines the constant part
	// of the maxLookahead
	lookaheadBuffer = build.Select(build.Var{
		Dev:      uint64(400),
		Standard: uint64(4000),
		Testing:  uint64(40),
	}).(uint64)

	// lookaheadRescanThreshold is the number of keys in the lookahead that will be
	// generated before a complete wallet rescan is initialized.
	lookaheadRescanThreshold = build.Select(build.Var{
		Dev:      uint64(100),
		Standard: uint64(1000),
		Testing:  uint64(10),
	}).(uint64)
)

// maxLookahead returns the size of the lookahead for a given seed progress
// which usually is the current primarySeedProgress
func maxLookahead(start uint64) uint64 {
	return start + lookaheadRescanThreshold + lookaheadBuffer + start/10
}
