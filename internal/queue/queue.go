package queue

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"wallet-api-server/internal/cache"
	"wallet-api-server/internal/db"
	"wallet-api-server/internal/models"
)

type WalletOpTask struct {
	Req  models.WalletOperationRequest
	Resp chan OpResult
}

type OpResult struct {
	Balance decimal.Decimal
	Err     error
	Msg     string
}

type QueueManager struct {
	queueMap   sync.Map // map[uuid.UUID]chan *WalletOpTask
	queueMutex sync.Mutex
	Cache      *cache.BalanceCache
}

func NewQueueManager(c *cache.BalanceCache) *QueueManager {
	return &QueueManager{Cache: c}
}

func (qm *QueueManager) getOrCreateQueue(walletId uuid.UUID) chan *WalletOpTask {
	ch, ok := qm.queueMap.Load(walletId)
	if ok {
		return ch.(chan *WalletOpTask)
	}
	qm.queueMutex.Lock()
	defer qm.queueMutex.Unlock()
	ch, ok = qm.queueMap.Load(walletId)
	if ok {
		return ch.(chan *WalletOpTask)
	}
	newCh := make(chan *WalletOpTask, 100)
	qm.queueMap.Store(walletId, newCh)
	go qm.walletWorker(walletId, newCh)
	return newCh
}

func (qm *QueueManager) walletWorker(walletId uuid.UUID, ch chan *WalletOpTask) {
	for task := range ch {
		balance, err, msg := processWalletOperation(task.Req)
		if err == nil {
			qm.Cache.Invalidate(walletId)
		}
		task.Resp <- OpResult{Balance: balance, Err: err, Msg: msg}
	}
}

func (qm *QueueManager) Enqueue(walletId uuid.UUID, req models.WalletOperationRequest) OpResult {
	ch := qm.getOrCreateQueue(walletId)
	respCh := make(chan OpResult)
	ch <- &WalletOpTask{Req: req, Resp: respCh}
	return <-respCh
}

func processWalletOperation(req models.WalletOperationRequest) (decimal.Decimal, error, string) {
	tx, err := db.DB.Begin(context.Background())
	if err != nil {
		return decimal.Zero, err, "Transaction error"
	}
	defer tx.Rollback(context.Background())

	var balance decimal.Decimal
	err = tx.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1 FOR UPDATE", req.WalletId).Scan(&balance)
	if err != nil {
		if err.Error() == "no rows in result set" {
			balance = decimal.Zero
			_, err = tx.Exec(context.Background(), "INSERT INTO wallets (wallet_id, balance) VALUES ($1, $2)", req.WalletId, balance)
			if err != nil {
				return decimal.Zero, err, "Failed to create wallet"
			}
		} else {
			return decimal.Zero, err, "Failed to read balance"
		}
	}

	switch req.OperationType {
	case models.DEPOSIT:
		balance = balance.Add(req.Amount)
	case models.WITHDRAW:
		if balance.LessThan(req.Amount) {
			return balance, fmt.Errorf("insufficient funds"), "Insufficient funds"
		}
		balance = balance.Sub(req.Amount)
	}

	_, err = tx.Exec(context.Background(), "UPDATE wallets SET balance=$1 WHERE wallet_id=$2", balance, req.WalletId)
	if err != nil {
		return decimal.Zero, err, "Failed to update balance"
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return decimal.Zero, err, "Transaction commit error"
	}

	return balance, nil, ""
}
