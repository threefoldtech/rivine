package blockcreator

import "time"

// SolveBlocks participates in the Proof Of Block Stake protocol by continously checking if
// unspent block stake outputs make a solution for the current unsolved block.
// If a match is found, the block is submitted to the consensus set.
// This function does not return until the blockcreator threadgroup is stopped.
func (bc *BlockCreator) SolveBlocks() {
	for {

		// Bail if 'Stop' has been called.
		select {
		case <-bc.tg.StopChan():
			return
		default:
		}

		// TODO: where to put the lock exactly
		// TODO: Take a copy here instead of in submitBlock?
		// Solve the block.
		// TODO: loop the solving blocks for the next 10 seconds
		// b, solved := bc.solveBlock()
		// if solved {
		// 	err := bc.submitBlock(b)
		// 	if err != nil {
		// 		bc.log.Println("ERROR: An error occurred while submitting a solved block:", err)
		// 	}
		// }
		//sleep a while before recalculating
		time.Sleep(8 * time.Second)
	}
}