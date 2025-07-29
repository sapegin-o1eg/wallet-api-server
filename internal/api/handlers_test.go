package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"wallet-api-server/internal/cache"
	"wallet-api-server/internal/db"
	"wallet-api-server/internal/queue"
)

type mockDBProvider struct{ mock.Mock }
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

func (m *mockRowScanner) Scan(dest ...interface{}) error {
	argsM := m.Called(dest)
	if argsM.Get(0) == nil {
		return nil
	}
	return argsM.Error(0)
}

func TestHandleWalletOperation_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &cache.BalanceCache{}
	q := &queue.QueueManager{Cache: c}
	mdb := new(mockDBProvider)
	h := NewHandler(c, q, mdb)
	r := gin.Default()
	r.POST("/wallet", h.HandleWalletOperation)
	w := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"invalid":1}`)
	req, _ := http.NewRequest("POST", "/wallet", body)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetBalance_InvalidUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &cache.BalanceCache{}
	q := &queue.QueueManager{Cache: c}
	mdb := new(mockDBProvider)
	h := NewHandler(c, q, mdb)
	r := gin.Default()
	r.GET("/wallet/:walletId", h.HandleGetBalance)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/wallet/invalid-uuid", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetBalance_CacheHit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &cache.BalanceCache{}
	id := uuid.New()
	c.Set(id, decimal.NewFromInt(100))
	q := &queue.QueueManager{Cache: c}
	mdb := new(mockDBProvider)
	h := NewHandler(c, q, mdb)
	r := gin.Default()
	r.GET("/wallet/:walletId", h.HandleGetBalance)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/wallet/"+id.String(), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	assert.Equal(t, true, resp["cached"])
	assert.Equal(t, id.String(), resp["walletId"].(string))
}

func TestHandleGetBalance_DB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &cache.BalanceCache{}
	q := &queue.QueueManager{Cache: c}
	mdb := new(mockDBProvider)
	mrow := new(mockRowScanner)
	id := uuid.New()
	mdb.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).Return(mrow)
	mrow.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		if dest, ok := args.Get(0).([]interface{}); ok {
			if len(dest) > 0 {
				if ptr, ok := dest[0].(*decimal.Decimal); ok {
					*ptr = decimal.NewFromInt(123)
				}
			}
		}
	})
	h := NewHandler(c, q, mdb)
	r := gin.Default()
	r.GET("/wallet/:walletId", h.HandleGetBalance)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/wallet/"+id.String(), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	assert.Equal(t, false, resp["cached"])
	assert.Equal(t, id.String(), resp["walletId"].(string))
	assert.Equal(t, "123", resp["balance"])
}
