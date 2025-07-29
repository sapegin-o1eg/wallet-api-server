package cache

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestBalanceCache_SetAndGet(t *testing.T) {
	c := &BalanceCache{}
	id := uuid.New()
	c.Set(id, decimal.NewFromInt(123))
	balance, ok := c.Get(id)
	assert.True(t, ok)
	assert.Equal(t, decimal.NewFromInt(123), balance)
}

func TestBalanceCache_Invalidate(t *testing.T) {
	c := &BalanceCache{}
	id := uuid.New()
	c.Set(id, decimal.NewFromInt(50))
	c.Invalidate(id)
	_, ok := c.Get(id)
	assert.False(t, ok)
}

func TestBalanceCache_Expiration(t *testing.T) {
	c := &BalanceCache{}
	id := uuid.New()
	c.Set(id, decimal.NewFromInt(77))
	// Force expiration
	entry, _ := c.m.Load(id)
	ce := entry.(*CacheEntry)
	ce.Expiration = time.Now().Add(-1 * time.Second)
	_, ok := c.Get(id)
	assert.False(t, ok)
}
