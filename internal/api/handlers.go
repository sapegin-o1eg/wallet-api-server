package api

import (
	"net/http"

	"wallet-api-server/internal/cache"
	"wallet-api-server/internal/db"
	"wallet-api-server/internal/models"
	"wallet-api-server/internal/queue"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	Cache *cache.BalanceCache
	Queue *queue.QueueManager
}

func NewHandler(c *cache.BalanceCache, q *queue.QueueManager) *Handler {
	return &Handler{Cache: c, Queue: q}
}

func (h *Handler) HandleWalletOperation(c *gin.Context) {
	var req models.WalletOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	walletUUID, err := uuid.Parse(req.WalletId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid walletId format"})
		return
	}
	res := h.Queue.Enqueue(walletUUID, req)
	if res.Err != nil {
		if res.Msg == "Insufficient funds" {
			c.JSON(http.StatusBadRequest, gin.H{"error": res.Msg})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": res.Msg})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"walletId": req.WalletId, "balance": res.Balance})
}

func (h *Handler) HandleGetBalance(c *gin.Context) {
	walletIdStr := c.Param("walletId")
	walletId, err := uuid.Parse(walletIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid walletId format"})
		return
	}
	if balance, ok := h.Cache.Get(walletId); ok {
		c.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance, "cached": true})
		return
	}
	var balance models.Wallet
	err = db.DB.QueryRow(c, "SELECT balance FROM wallets WHERE wallet_id=$1", walletId).Scan(&balance.Balance)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}
	h.Cache.Set(walletId, balance.Balance)
	c.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance.Balance, "cached": false})
}
