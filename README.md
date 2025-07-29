# Wallet API Server

This is a test task for the job interview.
It implements a high-performance wallet management API server built with Go, featuring concurrent transaction processing and caching.

## TL;DR

```bash
docker-compose up --build --detach --force-recreate
make test
```

## Features

- **Concurrent Operations**: Per-wallet queue system for thread-safe transactions
- **Caching**: Redis-like in-memory cache for balance queries
- **Database**: PostgreSQL with transaction support
- **API**: RESTful endpoints for wallet operations
- **Testing**: Comprehensive unit and integration tests

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL
- Docker
- make

### Environment Setup

Create `config.env`:
```env
DB_USER=your_user
DB_PASSWORD=your_password
DB_NAME=wallet_db
DB_HOST=localhost
DB_PORT=5432
HTTP_PORT=8080
HTTP_GET_WALLET_BALANCE_CACHE_TTL=10
```

### Run with Docker

```bash
docker-compose up -d
```

### Run Locally

```bash
go mod download
go run cmd/server/main.go
```

## API Endpoints

### Create/Update Wallet
```http
POST /api/v1/wallet
Content-Type: application/json

{
  "walletId": "uuid",
  "operationType": "DEPOSIT|WITHDRAW",
  "amount": "100.00"
}
```

### Get Balance
```http
GET /api/v1/wallets/{walletId}
```

## Testing

```bash
# Unit tests
go test ./...

# Integration tests
go test ./tests/
```

## Architecture

- **API Layer**: Gin HTTP server with request validation
- **Queue Layer**: Per-wallet worker queues for transaction serialization
- **Cache Layer**: In-memory balance cache with TTL
- **Database Layer**: PostgreSQL with transaction support
- **Models**: Structured data types with validation

## Performance

- Supports 1000+ concurrent operations per second
- Per-wallet transaction serialization prevents race conditions
- Configurable cache TTL for balance queries