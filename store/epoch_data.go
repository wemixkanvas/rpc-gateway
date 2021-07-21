package store

import (
	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	sdkerr "github.com/Conflux-Chain/go-conflux-sdk/types/errors"
	"github.com/conflux-chain/conflux-infura/metrics"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	emptyEpochData = EpochData{}
)

// EpochData wraps the blockchain data of an epoch.
type EpochData struct {
	Number   uint64         // epoch number
	Blocks   []*types.Block // blocks in order and the last one is pivot block
	Receipts map[types.Hash]*types.TransactionReceipt
}

// QueryEpochData queries blockchain data for the specified epoch number.
// TODO better to use batch API to return all if performance is low in case of high TPS.
func QueryEpochData(cfx sdk.ClientOperator, epochNumber uint64) (EpochData, error) {
	updater := metrics.NewTimerUpdaterByName("infura/duration/store/epoch/query")
	defer updater.Update()

	epoch := types.NewEpochNumberUint64(epochNumber)
	blockHashes, err := cfx.GetBlocksByEpoch(epoch)

	if err == nil && len(blockHashes) == 0 { // invalid epoch data, must have at lease one block
		err = errors.New("invalid epoch data (must have at least one block)")
	}

	if err != nil {
		return emptyEpochData, errors.WithMessagef(err, "failed to get blocks by epoch %v", epochNumber)
	}

	blocks := make([]*types.Block, 0, len(blockHashes))
	for _, hash := range blockHashes {
		block, err := cfx.GetBlockByHash(hash)
		if err != nil {
			return emptyEpochData, errors.WithMessagef(err, "failed to get block by hash %v", hash)
		}

		blocks = append(blocks, block)
	}

	// Batch get epoch receipts
	pivotHash := blockHashes[len(blockHashes)-1]
	epochReceipts, err := cfx.GetEpochReceiptsByPivotBlockHash(pivotHash)
	if err != nil {
		detected, errCode := sdkerr.DetectErrorCode(err)
		// The error is a business error and pivot hash assumption failed, must be pivot switched
		if sdkerr.IsBusinessError(err) && detected && (errCode == sdkerr.CodePivotAssumption || errCode == sdkerr.CodeBlockNotFound) {
			logrus.WithError(err).WithField("epochNo", epochNumber).Debug("Failed to query epoch data due to pivot switch")
			return emptyEpochData, errors.WithMessage(ErrEpochPivotSwitched, err.Error())
		}

		return emptyEpochData, err
	}

	receipts := make(map[types.Hash]*types.TransactionReceipt)
	var logIndex uint64

	for i, block := range blocks {
		for j, tx := range block.Transactions {
			logger := logrus.WithFields(logrus.Fields{
				"i": i, "j": j,
				"epoch": epochNumber,
				"block": block.Hash.String(),
				"tx":    tx.Hash.String(),
			})

			// skip unexecuted transaction, e.g.
			// 1) already executed in previous block
			// 2) never executed, e.g. nonce mismatch
			if tx.Status == nil || tx.BlockHash == nil {
				logger.Debug("Transaction not executed in block")
				continue
			}

			// Find transaction receipts from above batch retrieved receipts.
			// The order of batch retrieved receipts should be the same with the fetched blocks & transactions,
			// Even so, we'd better also do some bound checking and fault tolerance to be robust.
			if i >= len(epochReceipts) || j >= len(epochReceipts[i]) {
				logger.WithField("epochReceipts", epochReceipts).Error("Batch retrieved receipts out of bound")
				return emptyEpochData, errors.New("batch retrieved receipts out of bound")
			}

			receipt := &epochReceipts[i][j]
			if receipt == nil {
				logger.Error("Batch retrieved receipt not found")
				return emptyEpochData, errors.Errorf("batch retrieved receipt not found for tx %v", tx.Hash)
			}

			// Check receipt epoch number
			if uint64(*receipt.EpochNumber) != epochNumber {
				logger.Error("Batch retrieved receipt epoch number mismatch")
				return emptyEpochData, errors.Errorf("batch retrieved receipt epoch number mismatch, value = %v", uint64(*receipt.EpochNumber))
			}

			// Check receipt block hash
			if receipt.BlockHash.String() != block.Hash.String() {
				logger.Error("Batch retrieved receipt block hash mismatch")
				return emptyEpochData, errors.Errorf("batch retrieved receipt block hash mismatch, value = %v", receipt.BlockHash)
			}

			// Check receipt transaction hash
			if receipt.TransactionHash.String() != tx.Hash.String() {
				logger.Error("Batched retrieved receipt transaction hash mismatch")
				return emptyEpochData, errors.Errorf("batched retrieved receipt transaction hash mismatch, value = %v", receipt.TransactionHash)
			}

			// TODO enhance full node RPC to return receipts by block hash
			var txLogIndex uint64
			logs := make([]types.Log, 0, len(receipt.Logs))
			for _, log := range receipt.Logs {
				log.BlockHash = &receipt.BlockHash
				log.EpochNumber = types.NewBigInt(uint64(*receipt.EpochNumber))
				log.TransactionHash = &receipt.TransactionHash
				log.TransactionIndex = types.NewBigInt(uint64(receipt.Index))
				log.LogIndex = types.NewBigInt(logIndex)
				log.TransactionLogIndex = types.NewBigInt(txLogIndex)

				logs = append(logs, log)

				txLogIndex++
				logIndex++
			}
			receipt.Logs = logs // replace the origin logs

			receipts[tx.Hash] = receipt
		}
	}

	return EpochData{epochNumber, blocks, receipts}, nil
}
