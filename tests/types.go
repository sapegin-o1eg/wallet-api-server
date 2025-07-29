package tests

import (
	"github.com/shopspring/decimal"
)

type WalletOperationRequest struct {
	WalletId      string          `json:"walletId"`
	OperationType string          `json:"operationType"`
	Amount        decimal.Decimal `json:"amount"`
}

type WalletResponse struct {
	WalletId string          `json:"walletId"`
	Balance  decimal.Decimal `json:"balance"`
}

type BalanceResponse struct {
	WalletId string          `json:"walletId"`
	Balance  decimal.Decimal `json:"balance"`
	Cached   bool            `json:"cached"`
}
