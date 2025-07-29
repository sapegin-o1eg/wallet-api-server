package db

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
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

// DBProvider описывает интерфейс для работы с БД
// (или используйте testify/mock вручную)
//
//go:generate mockery --name=DBProvider --output=./mocks --case=underscore
type DBProvider interface {
	QueryRow(ctx context.Context, query string, args ...interface{}) RowScanner
	Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error)
	Begin(ctx context.Context) (TxProvider, error)
	Close()
}

type RowScanner interface {
	Scan(dest ...interface{}) error
}

type TxProvider interface {
	QueryRow(ctx context.Context, query string, args ...interface{}) RowScanner
	Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// PgxDBProvider реализует DBProvider поверх pgxpool.Pool

// Ensure PgxDBProvider implements DBProvider
var _ DBProvider = (*PgxDBProvider)(nil)

// Ensure PgxTxProvider implements TxProvider
var _ TxProvider = (*PgxTxProvider)(nil)

type PgxDBProvider struct {
	pool *pgxpool.Pool
}

type PgxTxProvider struct {
	tx pgx.Tx
}

type PgxRowScanner struct {
	r pgx.Row
}

func NewPgxDBProvider(pool *pgxpool.Pool) *PgxDBProvider {
	return &PgxDBProvider{pool: pool}
}

func (p *PgxDBProvider) QueryRow(ctx context.Context, query string, args ...interface{}) RowScanner {
	return &PgxRowScanner{r: p.pool.QueryRow(ctx, query, args...)}
}

func (p *PgxDBProvider) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	return p.pool.Exec(ctx, query, args...)
}

func (p *PgxDBProvider) Begin(ctx context.Context) (TxProvider, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &PgxTxProvider{tx: tx}, nil
}

func (p *PgxDBProvider) Close() {
	p.pool.Close()
}

func (t *PgxTxProvider) QueryRow(ctx context.Context, query string, args ...interface{}) RowScanner {
	return &PgxRowScanner{r: t.tx.QueryRow(ctx, query, args...)}
}

func (t *PgxTxProvider) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	return t.tx.Exec(ctx, query, args...)
}

func (t *PgxTxProvider) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t *PgxTxProvider) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

func (r *PgxRowScanner) Scan(dest ...interface{}) error {
	return r.r.Scan(dest...)
}
