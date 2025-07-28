package cache

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
)

type CacheEntry struct {
	Balance    decimal.Decimal
	Expiration time.Time
}

type BalanceCache struct {
	m sync.Map // map[uuid.UUID]*CacheEntry
}

func getCacheTTL() time.Duration {
	viper.AutomaticEnv()
	viper.SetDefault("HTTP_GET_WALLET_BALANCE_CACHE_TTL", 10)
	ttl := viper.GetInt("HTTP_GET_WALLET_BALANCE_CACHE_TTL")
	return time.Duration(ttl) * time.Second
}

var cacheTTL = getCacheTTL()

func (c *BalanceCache) Invalidate(walletId uuid.UUID) {
	c.m.Delete(walletId)
}

func (c *BalanceCache) Set(walletId uuid.UUID, balance decimal.Decimal) {
	c.m.Store(walletId, &CacheEntry{
		Balance:    balance,
		Expiration: time.Now().Add(cacheTTL),
	})
}

func (c *BalanceCache) Get(walletId uuid.UUID) (decimal.Decimal, bool) {
	v, found := c.m.Load(walletId)
	if !found {
		return decimal.Zero, false
	}
	entry := v.(*CacheEntry)
	if time.Now().After(entry.Expiration) {
		c.m.Delete(walletId)
		return decimal.Zero, false
	}
	return entry.Balance, true
}
