package db

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
)

var DB *pgxpool.Pool

func InitDB() {
	viper.AutomaticEnv()
	dbUser := viper.GetString("DB_USER")
	dbPassword := viper.GetString("DB_PASSWORD")
	dbName := viper.GetString("DB_NAME")
	dbHost := viper.GetString("DB_HOST")
	dbPort := viper.GetString("DB_PORT")

	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
	var err error
	DB, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
}

func CloseDB() {
	if DB != nil {
		DB.Close()
	}
}

func CreateTablesIfNotExist() error {
	query := `
	CREATE TABLE IF NOT EXISTS wallets (
		wallet_id UUID PRIMARY KEY,
		balance NUMERIC(19,4) NOT NULL DEFAULT 0
	);
	`
	_, err := DB.Exec(context.Background(), query)
	return err
}
