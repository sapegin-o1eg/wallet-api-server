package models

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
