package queue

import (
	"context"
	"fmt"
	"log"
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
	DB         db.DBProvider
}

func NewQueueManager(c *cache.BalanceCache, dbProvider db.DBProvider) *QueueManager {
	return &QueueManager{Cache: c, DB: dbProvider}
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
		balance, msg, err := processWalletOperation(qm.DB, task.Req)
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

func processWalletOperation(dbProvider db.DBProvider, req models.WalletOperationRequest) (decimal.Decimal, string, error) {
	tx, err := dbProvider.Begin(context.Background())
	if err != nil {
		return decimal.Zero, "Transaction error", err
	}

	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil {
				log.Printf("Failed to rollback transaction: %v", rollbackErr)
			}
		}
	}()

	var balance decimal.Decimal
	err = tx.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1 FOR UPDATE", req.WalletId).Scan(&balance)
	if err != nil {
		if err.Error() == "no rows in result set" {
			balance = decimal.Zero
			_, err = tx.Exec(context.Background(), "INSERT INTO wallets (wallet_id, balance) VALUES ($1, $2)", req.WalletId, balance)
			if err != nil {
				return decimal.Zero, "Failed to create wallet", err
			}
		} else {
			return decimal.Zero, "Failed to read balance", err
		}
	}

	switch req.OperationType {
	case models.DEPOSIT:
		balance = balance.Add(req.Amount)
	case models.WITHDRAW:
		if balance.LessThan(req.Amount) {
			return balance, "Insufficient funds", fmt.Errorf("insufficient funds")
		}
		balance = balance.Sub(req.Amount)
	}

	_, err = tx.Exec(context.Background(), "UPDATE wallets SET balance=$1 WHERE wallet_id=$2", balance, req.WalletId)
	if err != nil {
		return decimal.Zero, "Failed to update balance", err
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return decimal.Zero, "Transaction commit error", err
	}

	committed = true
	return balance, "", nil
}
