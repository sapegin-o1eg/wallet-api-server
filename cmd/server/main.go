package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
)

type OperationType string

const (
	DEPOSIT  OperationType = "DEPOSIT"
	WITHDRAW OperationType = "WITHDRAW"
)

type WalletOperationRequest struct {
	WalletId      string          `json:"walletId" binding:"required,uuid"`
	OperationType OperationType   `json:"operationType" binding:"required,oneof=DEPOSIT WITHDRAW"`
	Amount        decimal.Decimal `json:"amount" binding:"required,gt=0"`
}

type Wallet struct {
	WalletId uuid.UUID       `json:"walletId"`
	Balance  decimal.Decimal `json:"balance"`
}

var db *pgxpool.Pool

// Thread-safe cache for wallet balances
// Key — walletId, value — CacheEntry
var balanceCache struct {
	m sync.Map // map[uuid.UUID]*CacheEntry
}

// CacheEntry represents a cached balance with expiration
// Used for TTL and manual invalidation
type CacheEntry struct {
	Balance    decimal.Decimal
	Expiration time.Time
}

// Cache TTL duration
func getCacheTTL() time.Duration {
	viper.AutomaticEnv()
	viper.SetDefault("HTTP_GET_WALLET_BALANCE_CACHE_TTL", 10)
	ttl := viper.GetInt("HTTP_GET_WALLET_BALANCE_CACHE_TTL")
	return time.Duration(ttl) * time.Second
}

var cacheTTL = getCacheTTL()

// InvalidateBalanceCache removes cache for walletId
func InvalidateBalanceCache(walletId uuid.UUID) {
	balanceCache.m.Delete(walletId)
}

// SetBalanceCache sets balance for walletId with TTL
func SetBalanceCache(walletId uuid.UUID, balance decimal.Decimal) {
	balanceCache.m.Store(walletId, &CacheEntry{
		Balance:    balance,
		Expiration: time.Now().Add(cacheTTL),
	})
}

// GetBalanceCache returns cached balance if valid, else ok=false
func GetBalanceCache(walletId uuid.UUID) (balance decimal.Decimal, ok bool) {
	v, found := balanceCache.m.Load(walletId)
	if !found {
		return decimal.Zero, false
	}
	entry := v.(*CacheEntry)
	if time.Now().After(entry.Expiration) {
		balanceCache.m.Delete(walletId)
		return decimal.Zero, false
	}
	return entry.Balance, true
}

// --- Operation Queue ---
// For sequential processing of operations by wallet
// Map: walletId -> chan *walletOpTask
var (
	queueMap   sync.Map // map[uuid.UUID]chan *walletOpTask
	queueMutex sync.Mutex
)

type walletOpTask struct {
	ctx  *gin.Context
	req  WalletOperationRequest
	resp chan opResult
}

type opResult struct {
	balance decimal.Decimal
	err     error
	msg     string
}

// getOrCreateQueue returns channel for walletId, creates if not exists
func getOrCreateQueue(walletId uuid.UUID) chan *walletOpTask {
	ch, ok := queueMap.Load(walletId)
	if ok {
		return ch.(chan *walletOpTask)
	}
	queueMutex.Lock()
	defer queueMutex.Unlock()
	// Double-check after locking
	ch, ok = queueMap.Load(walletId)
	if ok {
		return ch.(chan *walletOpTask)
	}
	newCh := make(chan *walletOpTask, 100)
	queueMap.Store(walletId, newCh)
	go walletWorker(walletId, newCh)
	return newCh
}

// walletWorker processes operations for a single wallet sequentially
func walletWorker(walletId uuid.UUID, ch chan *walletOpTask) {
	for task := range ch {
		balance, err, msg := processWalletOperation(task.req)
		if err == nil {
			InvalidateBalanceCache(walletId) // Invalidate cache on balance change
		}
		task.resp <- opResult{balance: balance, err: err, msg: msg}
	}
}

// processWalletOperation performs DB logic, returns balance/error/message
func processWalletOperation(req WalletOperationRequest) (decimal.Decimal, error, string) {
	tx, err := db.Begin(context.Background())
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
	case DEPOSIT:
		balance = balance.Add(req.Amount)
	case WITHDRAW:
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

func main() {
	// Load configuration using viper
	viper.SetConfigFile("config.env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: Cant read config.env, ENV variables will be used: %v", err)
	}

	dbUser := viper.GetString("DB_USER")
	dbPassword := viper.GetString("DB_PASSWORD")
	dbName := viper.GetString("DB_NAME")
	dbHost := viper.GetString("DB_HOST")
	dbPort := viper.GetString("DB_PORT")

	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
	var err error
	db, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	if err := createTablesIfNotExist(); err != nil {
		log.Fatalf("Initializing DB tables error: %v", err)
	}

	r := gin.Default()

	r.POST("/api/v1/wallet", handleWalletOperation)
	r.GET("/api/v1/wallets/:walletId", handleGetBalance)

	viper.SetDefault("HTTP_PORT", "8080")
	httpPort := viper.GetString("HTTP_PORT")
	r.Run(fmt.Sprintf(":%v", httpPort))
}

func createTablesIfNotExist() error {
	query := `
	CREATE TABLE IF NOT EXISTS wallets (
		wallet_id UUID PRIMARY KEY,
		balance NUMERIC(19,4) NOT NULL DEFAULT 0
	);
	`
	_, err := db.Exec(context.Background(), query)
	return err
}

func handleWalletOperation(c *gin.Context) {
	var req WalletOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Check if WalletId is uuid.UUID
	walletUUID, err := uuid.Parse(req.WalletId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid walletId format"})
		return
	}
	// Push operation to queue for this wallet
	ch := getOrCreateQueue(walletUUID)
	respCh := make(chan opResult)
	ch <- &walletOpTask{ctx: c, req: req, resp: respCh}
	res := <-respCh
	if res.err != nil {
		if res.msg == "Insufficient funds" {
			c.JSON(http.StatusBadRequest, gin.H{"error": res.msg})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": res.msg})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"walletId": req.WalletId, "balance": res.balance})
}

func handleGetBalance(c *gin.Context) {
	walletIdStr := c.Param("walletId")
	walletId, err := uuid.Parse(walletIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid walletId format"})
		return
	}
	if balance, ok := GetBalanceCache(walletId); ok {
		c.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance, "cached": true})
		return
	}
	var balance decimal.Decimal
	err = db.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1", walletId).Scan(&balance)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}
	SetBalanceCache(walletId, balance)
	c.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance, "cached": false})
}
