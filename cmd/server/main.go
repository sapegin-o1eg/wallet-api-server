package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
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
	WalletId string          `json:"walletId"`
	Balance  decimal.Decimal `json:"balance"`
}

var db *pgxpool.Pool

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

	r.Run(":8080")
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

	tx, err := db.Begin(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction error"})
		return
	}
	defer tx.Rollback(context.Background())

	var balance decimal.Decimal
	err = tx.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1 FOR UPDATE", req.WalletId).Scan(&balance)
	if err != nil {
		if err.Error() == "no rows in result set" {
			// Create wallet if not exists
			balance = decimal.Zero
			_, err = tx.Exec(context.Background(), "INSERT INTO wallets (wallet_id, balance) VALUES ($1, $2)", req.WalletId, balance)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create wallet"})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read balance"})
			return
		}
	}

	switch req.OperationType {
	case DEPOSIT:
		balance = balance.Add(req.Amount)
	case WITHDRAW:
		if balance.LessThan(req.Amount) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient funds"})
			return
		}
		balance = balance.Sub(req.Amount)
	}

	_, err = tx.Exec(context.Background(), "UPDATE wallets SET balance=$1 WHERE wallet_id=$2", balance, req.WalletId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update balance"})
		return
	}

	err = tx.Commit(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction commit error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"walletId": req.WalletId, "balance": balance})
}

func handleGetBalance(c *gin.Context) {
	walletId := c.Param("walletId")
	var balance decimal.Decimal
	err := db.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1", walletId).Scan(&balance)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance})
}
