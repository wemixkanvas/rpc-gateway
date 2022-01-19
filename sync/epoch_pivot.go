package sync

import (
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	citypes "github.com/conflux-chain/conflux-infura/types"
	"github.com/pkg/errors"
)

// epochPivotWindow caches epoch pivot info with limited capacity.
type epochPivotWindow struct {
	// hashmap to cache pivot hash of epoch (epoch number => pivot block hash)
	epochToPivotHash map[uint64]types.Hash
	// maximum number of epochs to hold
	capacity uint32
	// cached epoch range
	epochFrom, epochTo uint64
}

func newEpochPivotWindow(capacity uint32) *epochPivotWindow {
	win := &epochPivotWindow{capacity: capacity}
	win.reset()

	return win
}

func (win *epochPivotWindow) getPivotHash(epoch uint64) (types.Hash, bool) {
	pivotHash, ok := win.epochToPivotHash[epoch]
	return pivotHash, ok
}

func (win *epochPivotWindow) reset() {
	win.epochFrom = citypes.EpochNumberNil
	win.epochTo = citypes.EpochNumberNil

	win.epochToPivotHash = make(map[uint64]types.Hash)
}

func (win *epochPivotWindow) push(pivotBlock *types.Block) error {
	pivotEpochNum := pivotBlock.EpochNumber.ToInt().Uint64()

	if win.size() != 0 && (win.epochTo+1) != pivotEpochNum {
		return errors.Errorf(
			"incontinuous epoch pushed, expect %v got %v", win.epochTo+1, pivotEpochNum,
		)
	}

	// reclaim in case of memory blast
	for win.size() != 0 && win.size() >= win.capacity {
		delete(win.epochToPivotHash, win.epochFrom)
		win.epochFrom++
	}

	// cache store epoch pivot hash
	win.epochToPivotHash[pivotEpochNum] = pivotBlock.Hash
	if !win.isSet() {
		win.epochFrom, win.epochTo = pivotEpochNum, pivotEpochNum
	} else {
		win.epochTo = pivotEpochNum
	}

	return nil
}

func (win *epochPivotWindow) popn(epochUntil uint64) {
	if win.size() == 0 || win.epochTo < epochUntil {
		return
	}

	for win.epochTo >= epochUntil {
		delete(win.epochToPivotHash, win.epochTo)
		win.epochTo--

		if win.size() == 0 {
			win.reset()
			return
		}
	}
}

func (win *epochPivotWindow) isSet() bool {
	return win.epochFrom != citypes.EpochNumberNil && win.epochTo != citypes.EpochNumberNil
}

func (win *epochPivotWindow) size() uint32 {
	if !win.isSet() || win.epochFrom > win.epochTo {
		return 0
	}

	return uint32(win.epochTo - win.epochFrom + 1)
}
