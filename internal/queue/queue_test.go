package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"wallet-api-server/internal/cache"
	"wallet-api-server/internal/db"
	"wallet-api-server/internal/models"
)

// mockDBProvider implements db.DBProvider for tests
// mockTxProvider implements db.TxProvider for tests
// mockRowScanner implements db.RowScanner for tests

type mockDBProvider struct{ mock.Mock }

type mockTxProvider struct{ mock.Mock }
type mockRowScanner struct{ mock.Mock }

func (m *mockDBProvider) QueryRow(ctx context.Context, query string, args ...interface{}) db.RowScanner {
	argsM := m.Called(ctx, query, args)
	return argsM.Get(0).(db.RowScanner)
}
func (m *mockDBProvider) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	argsM := m.Called(ctx, query, args)
	return argsM.Get(0), argsM.Error(1)
}
func (m *mockDBProvider) Begin(ctx context.Context) (db.TxProvider, error) {
	argsM := m.Called(ctx)
	return argsM.Get(0).(db.TxProvider), argsM.Error(1)
}
func (m *mockDBProvider) Close() {}

func (m *mockTxProvider) QueryRow(ctx context.Context, query string, args ...interface{}) db.RowScanner {
	argsM := m.Called(ctx, query, args)
	return argsM.Get(0).(db.RowScanner)
}
func (m *mockTxProvider) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	argsM := m.Called(ctx, query, args)
	return argsM.Get(0), argsM.Error(1)
}
func (m *mockTxProvider) Commit(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *mockTxProvider) Rollback(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *mockRowScanner) Scan(dest ...interface{}) error {
	argsM := m.Called(dest)
	if argsM.Get(0) == nil {
		return nil
	}
	return argsM.Error(0)
}

func TestQueueManager_Enqueue_NewWallet(t *testing.T) {
	c := &cache.BalanceCache{}
	mdb := new(mockDBProvider)
	mtx := new(mockTxProvider)
	mrow := new(mockRowScanner)

	id := uuid.New()
	request := models.WalletOperationRequest{
		WalletId:      id.String(),
		OperationType: models.DEPOSIT,
		Amount:        decimal.NewFromInt(100),
	}

	mdb.On("Begin", mock.Anything).Return(mtx, nil)
	mtx.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).Return(mrow)
	mrow.On("Scan", mock.Anything).Return(errors.New("no rows in result set"))
	mtx.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mtx.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mtx.On("Commit", mock.Anything).Return(nil)
	mtx.On("Rollback", mock.Anything).Return(nil)

	qm := NewQueueManager(c, mdb)
	res := qm.Enqueue(id, request)
	assert.True(t, res.Balance.GreaterThanOrEqual(decimal.Zero))
}

func TestProcessWalletOperation_InsufficientFunds(t *testing.T) {
	mdb := new(mockDBProvider)
	mtx := new(mockTxProvider)
	mrow := new(mockRowScanner)

	mdb.On("Begin", mock.Anything).Return(mtx, nil)
	mtx.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).Return(mrow)
	mrow.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		if dest, ok := args.Get(0).([]interface{}); ok {
			if len(dest) > 0 {
				if ptr, ok := dest[0].(*decimal.Decimal); ok {
					*ptr = decimal.NewFromInt(10)
				}
			}
		}
	})
	mtx.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mtx.On("Commit", mock.Anything).Return(nil)
	mtx.On("Rollback", mock.Anything).Return(nil)

	id := uuid.New().String()
	request := models.WalletOperationRequest{
		WalletId:      id,
		OperationType: models.WITHDRAW,
		Amount:        decimal.NewFromInt(1000),
	}
	balance, msg, err := processWalletOperation(mdb, request)
	assert.Error(t, err)
	assert.Equal(t, "Insufficient funds", msg)
	assert.True(t, balance.LessThan(decimal.NewFromInt(1000)))
}

func TestQueueManager_getOrCreateQueue(t *testing.T) {
	c := &cache.BalanceCache{}
	mdb := new(mockDBProvider)
	qm := NewQueueManager(c, mdb)
	id := uuid.New()
	ch := qm.getOrCreateQueue(id)
	assert.NotNil(t, ch)
	ch2 := qm.getOrCreateQueue(id)
	assert.Equal(t, ch, ch2)
}
