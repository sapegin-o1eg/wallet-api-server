package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"wallet-api-server/internal/api"
	"wallet-api-server/internal/cache"
	"wallet-api-server/internal/db"
	"wallet-api-server/internal/queue"
)

func main() {
	viper.SetConfigFile("config.env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: Cant read config.env, ENV variables will be used: %v", err)
	}

	db.InitDB()
	defer db.CloseDB()

	dbProvider := db.NewPgxDBProvider(db.DB)

	if err := db.CreateTablesIfNotExist(); err != nil {
		log.Fatalf("Initializing DB tables error: %v", err)
	}

	cacheInstance := &cache.BalanceCache{}
	queueManager := queue.NewQueueManager(cacheInstance, dbProvider)
	handler := api.NewHandler(cacheInstance, queueManager, dbProvider)

	r := gin.Default()
	r.POST("/api/v1/wallet", handler.HandleWalletOperation)
	r.GET("/api/v1/wallets/:walletId", handler.HandleGetBalance)

	viper.SetDefault("HTTP_PORT", "8080")
	httpPort := viper.GetString("HTTP_PORT")
	if err := r.Run(":" + httpPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
