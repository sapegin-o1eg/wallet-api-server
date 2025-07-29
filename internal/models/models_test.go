package models

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestOperationTypeConstants(t *testing.T) {
	assert.Equal(t, OperationType("DEPOSIT"), DEPOSIT)
	assert.Equal(t, OperationType("WITHDRAW"), WITHDRAW)
}

func TestWalletOperationRequest_Struct(t *testing.T) {
	req := WalletOperationRequest{
		WalletId:      uuid.New().String(),
		OperationType: DEPOSIT,
		Amount:        decimal.NewFromInt(10),
	}
	assert.Equal(t, DEPOSIT, req.OperationType)
	assert.True(t, req.Amount.Equal(decimal.NewFromInt(10)))
}

func TestWallet_Struct(t *testing.T) {
	id := uuid.New()
	w := Wallet{
		WalletId: id,
		Balance:  decimal.NewFromInt(100),
	}
	assert.Equal(t, id, w.WalletId)
	assert.True(t, w.Balance.Equal(decimal.NewFromInt(100)))
}
