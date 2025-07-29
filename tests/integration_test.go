package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletConcurrency(t *testing.T) {
	// Load config from ../../config.env
	configPath := filepath.Join("..", "config.env")
	viper.SetConfigFile(configPath)
	viper.SetConfigType("env")
	require.NoError(t, viper.ReadInConfig())

	// Server URL - assuming server is running on default port
	serverURL := "http://localhost:8080"

	// Test wallet ID
	walletID := uuid.New().String()

	// Initial deposit amount: $1,000,000
	initialAmount := decimal.NewFromInt(1000000)

	// Small operation amount: $1
	smallAmount := decimal.NewFromInt(1)

	// Number of concurrent operations
	numOperations := 5000

	// Rate limiting: max 1000 RPS
	maxRPS := 1000
	rateLimiter := time.NewTicker(time.Second / time.Duration(maxRPS))
	defer rateLimiter.Stop()

	t.Logf("Starting integration test with wallet ID: %s", walletID)
	t.Logf("Initial deposit amount: $%s", initialAmount.String())
	t.Logf("Number of concurrent operations: %d", numOperations*2) // 5000 withdraw + 5000 deposit
	t.Logf("Rate limit: %d RPS", maxRPS)

	// Step 1: Initial deposit of $1,000,000
	t.Log("Step 1: Making initial deposit of $1,000,000")

	initialReq := WalletOperationRequest{
		WalletId:      walletID,
		OperationType: "DEPOSIT",
		Amount:        initialAmount,
	}

	reqBody, err := json.Marshal(initialReq)
	require.NoError(t, err)

	resp, err := http.Post(serverURL+"/api/v1/wallet", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var initialResp WalletResponse
	err = json.NewDecoder(resp.Body).Decode(&initialResp)
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Logf("Error closing response body: %v", closeErr)
	}
	require.NoError(t, err)

	assert.Equal(t, walletID, initialResp.WalletId)
	assert.Equal(t, initialAmount, initialResp.Balance)

	t.Logf("Initial deposit successful. Balance: $%s", initialResp.Balance.String())

	// Step 2: Verify initial balance
	t.Log("Step 2: Verifying initial balance")

	resp, err = http.Get(serverURL + "/api/v1/wallets/" + walletID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var balanceResp BalanceResponse
	err = json.NewDecoder(resp.Body).Decode(&balanceResp)
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Logf("Error closing response body: %v", closeErr)
	}
	require.NoError(t, err)

	assert.Equal(t, walletID, balanceResp.WalletId)
	assert.Equal(t, initialAmount, balanceResp.Balance)

	t.Logf("Balance verification successful. Balance: $%s", balanceResp.Balance.String())

	// Step 3: Concurrent operations with rate limiting
	t.Log("Step 3: Starting concurrent operations with rate limiting")

	var wg sync.WaitGroup
	errorCount := 0
	successCount := 0
	var mu sync.Mutex

	// Function to make a wallet operation
	makeOperation := func(operationType string, amount decimal.Decimal) {
		defer wg.Done()

		// Wait for rate limiter
		<-rateLimiter.C

		req := WalletOperationRequest{
			WalletId:      walletID,
			OperationType: operationType,
			Amount:        amount,
		}

		reqBody, err := json.Marshal(req)
		if err != nil {
			mu.Lock()
			errorCount++
			t.Logf("Error marshaling request: %v", err)
			mu.Unlock()
			return
		}

		resp, err := http.Post(serverURL+"/api/v1/wallet", "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			mu.Lock()
			errorCount++
			t.Logf("Error making request: %v", err)
			mu.Unlock()
			return
		}

		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Logf("Error closing response body: %v", closeErr)
			}
		}()

		mu.Lock()
		if resp.StatusCode == http.StatusOK {
			successCount++
		} else {
			errorCount++
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Request failed with status %d: %s", resp.StatusCode, string(body))
		}
		mu.Unlock()
	}

	// Start 5000 withdraw operations
	t.Log("Starting 5000 withdraw operations")
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go makeOperation("WITHDRAW", smallAmount)
	}

	// Start 5000 deposit operations
	t.Log("Starting 5000 deposit operations")
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go makeOperation("DEPOSIT", smallAmount)
	}

	// Wait for all operations to complete
	t.Log("Waiting for all operations to complete...")
	wg.Wait()

	t.Logf("Operations completed. Success: %d, Errors: %d", successCount, errorCount)

	// Step 4: Verify final balance
	t.Log("Step 4: Verifying final balance")

	// Wait a bit for all operations to be processed
	time.Sleep(2 * time.Second)

	resp, err = http.Get(serverURL + "/api/v1/wallets/" + walletID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&balanceResp)
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Logf("Error closing response body: %v", closeErr)
	}
	require.NoError(t, err)

	assert.Equal(t, walletID, balanceResp.WalletId)
	assert.Equal(t, initialAmount, balanceResp.Balance,
		"Final balance should equal initial balance. Expected: %s, Got: %s",
		initialAmount.String(), balanceResp.Balance.String())

	t.Logf("Final balance verification successful. Balance: $%s", balanceResp.Balance.String())

	// Step 5: Assertions
	t.Log("Step 5: Final assertions")

	// All requests should have succeeded (no errors)
	assert.Equal(t, 0, errorCount, "No requests should have failed")
	assert.Equal(t, numOperations*2, successCount, "All operations should have succeeded")

	// Final balance should equal initial balance
	assert.Equal(t, initialAmount, balanceResp.Balance,
		"Final balance should equal initial balance after equal number of deposits and withdrawals")

	t.Log("Integration test completed successfully!")
}
