[1mdiff --git a/cmd/server/main.go b/cmd/server/main.go[m
[1mindex fc0b4a0..536c0e6 100644[m
[1m--- a/cmd/server/main.go[m
[1m+++ b/cmd/server/main.go[m
[36m@@ -6,7 +6,11 @@[m [mimport ([m
 	"log"[m
 	"net/http"[m
 [m
[32m+[m	[32m"sync"[m
[32m+[m	[32m"time"[m
[32m+[m
 	"github.com/gin-gonic/gin"[m
[32m+[m	[32m"github.com/google/uuid"[m
 	"github.com/jackc/pgx/v5/pgxpool"[m
 	"github.com/shopspring/decimal"[m
 	"github.com/spf13/viper"[m
[36m@@ -26,12 +30,157 @@[m [mtype WalletOperationRequest struct {[m
 }[m
 [m
 type Wallet struct {[m
[31m-	WalletId string          `json:"walletId"`[m
[32m+[m	[32mWalletId uuid.UUID       `json:"walletId"`[m
 	Balance  decimal.Decimal `json:"balance"`[m
 }[m
 [m
 var db *pgxpool.Pool[m
 [m
[32m+[m[32m// Thread-safe cache for wallet balances[m
[32m+[m[32m// Key — walletId, value — CacheEntry[m
[32m+[m[32mvar balanceCache struct {[m
[32m+[m	[32mm sync.Map // map[uuid.UUID]*CacheEntry[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// CacheEntry represents a cached balance with expiration[m
[32m+[m[32m// Used for TTL and manual invalidation[m
[32m+[m[32mtype CacheEntry struct {[m
[32m+[m	[32mBalance    decimal.Decimal[m
[32m+[m	[32mExpiration time.Time[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// Cache TTL duration[m
[32m+[m[32mfunc getCacheTTL() time.Duration {[m
[32m+[m	[32mviper.AutomaticEnv()[m
[32m+[m	[32mviper.SetDefault("HTTP_GET_WALLET_BALANCE_CACHE_TTL", 10)[m
[32m+[m	[32mttl := viper.GetInt("HTTP_GET_WALLET_BALANCE_CACHE_TTL")[m
[32m+[m	[32mreturn time.Duration(ttl) * time.Second[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32mvar cacheTTL = getCacheTTL()[m
[32m+[m
[32m+[m[32m// InvalidateBalanceCache removes cache for walletId[m
[32m+[m[32mfunc InvalidateBalanceCache(walletId uuid.UUID) {[m
[32m+[m	[32mbalanceCache.m.Delete(walletId)[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// SetBalanceCache sets balance for walletId with TTL[m
[32m+[m[32mfunc SetBalanceCache(walletId uuid.UUID, balance decimal.Decimal) {[m
[32m+[m	[32mbalanceCache.m.Store(walletId, &CacheEntry{[m
[32m+[m		[32mBalance:    balance,[m
[32m+[m		[32mExpiration: time.Now().Add(cacheTTL),[m
[32m+[m	[32m})[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// GetBalanceCache returns cached balance if valid, else ok=false[m
[32m+[m[32mfunc GetBalanceCache(walletId uuid.UUID) (balance decimal.Decimal, ok bool) {[m
[32m+[m	[32mv, found := balanceCache.m.Load(walletId)[m
[32m+[m	[32mif !found {[m
[32m+[m		[32mreturn decimal.Zero, false[m
[32m+[m	[32m}[m
[32m+[m	[32mentry := v.(*CacheEntry)[m
[32m+[m	[32mif time.Now().After(entry.Expiration) {[m
[32m+[m		[32mbalanceCache.m.Delete(walletId)[m
[32m+[m		[32mreturn decimal.Zero, false[m
[32m+[m	[32m}[m
[32m+[m	[32mreturn entry.Balance, true[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// --- Operation Queue ---[m
[32m+[m[32m// For sequential processing of operations by wallet[m
[32m+[m[32m// Map: walletId -> chan *walletOpTask[m
[32m+[m[32mvar ([m
[32m+[m	[32mqueueMap   sync.Map // map[uuid.UUID]chan *walletOpTask[m
[32m+[m	[32mqueueMutex sync.Mutex[m
[32m+[m[32m)[m
[32m+[m
[32m+[m[32mtype walletOpTask struct {[m
[32m+[m	[32mctx  *gin.Context[m
[32m+[m	[32mreq  WalletOperationRequest[m
[32m+[m	[32mresp chan opResult[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32mtype opResult struct {[m
[32m+[m	[32mbalance decimal.Decimal[m
[32m+[m	[32merr     error[m
[32m+[m	[32mmsg     string[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// getOrCreateQueue returns channel for walletId, creates if not exists[m
[32m+[m[32mfunc getOrCreateQueue(walletId uuid.UUID) chan *walletOpTask {[m
[32m+[m	[32mch, ok := queueMap.Load(walletId)[m
[32m+[m	[32mif ok {[m
[32m+[m		[32mreturn ch.(chan *walletOpTask)[m
[32m+[m	[32m}[m
[32m+[m	[32mqueueMutex.Lock()[m
[32m+[m	[32mdefer queueMutex.Unlock()[m
[32m+[m	[32m// Double-check after locking[m
[32m+[m	[32mch, ok = queueMap.Load(walletId)[m
[32m+[m	[32mif ok {[m
[32m+[m		[32mreturn ch.(chan *walletOpTask)[m
[32m+[m	[32m}[m
[32m+[m	[32mnewCh := make(chan *walletOpTask, 100)[m
[32m+[m	[32mqueueMap.Store(walletId, newCh)[m
[32m+[m	[32mgo walletWorker(walletId, newCh)[m
[32m+[m	[32mreturn newCh[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// walletWorker processes operations for a single wallet sequentially[m
[32m+[m[32mfunc walletWorker(walletId uuid.UUID, ch chan *walletOpTask) {[m
[32m+[m	[32mfor task := range ch {[m
[32m+[m		[32mbalance, err, msg := processWalletOperation(task.req)[m
[32m+[m		[32mif err == nil {[m
[32m+[m			[32mInvalidateBalanceCache(walletId) // Invalidate cache on balance change[m
[32m+[m		[32m}[m
[32m+[m		[32mtask.resp <- opResult{balance: balance, err: err, msg: msg}[m
[32m+[m	[32m}[m
[32m+[m[32m}[m
[32m+[m
[32m+[m[32m// processWalletOperation performs DB logic, returns balance/error/message[m
[32m+[m[32mfunc processWalletOperation(req WalletOperationRequest) (decimal.Decimal, error, string) {[m
[32m+[m	[32mtx, err := db.Begin(context.Background())[m
[32m+[m	[32mif err != nil {[m
[32m+[m		[32mreturn decimal.Zero, err, "Transaction error"[m
[32m+[m	[32m}[m
[32m+[m	[32mdefer tx.Rollback(context.Background())[m
[32m+[m
[32m+[m	[32mvar balance decimal.Decimal[m
[32m+[m	[32merr = tx.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1 FOR UPDATE", req.WalletId).Scan(&balance)[m
[32m+[m	[32mif err != nil {[m
[32m+[m		[32mif err.Error() == "no rows in result set" {[m
[32m+[m			[32mbalance = decimal.Zero[m
[32m+[m			[32m_, err = tx.Exec(context.Background(), "INSERT INTO wallets (wallet_id, balance) VALUES ($1, $2)", req.WalletId, balance)[m
[32m+[m			[32mif err != nil {[m
[32m+[m				[32mreturn decimal.Zero, err, "Failed to create wallet"[m
[32m+[m			[32m}[m
[32m+[m		[32m} else {[m
[32m+[m			[32mreturn decimal.Zero, err, "Failed to read balance"[m
[32m+[m		[32m}[m
[32m+[m	[32m}[m
[32m+[m
[32m+[m	[32mswitch req.OperationType {[m
[32m+[m	[32mcase DEPOSIT:[m
[32m+[m		[32mbalance = balance.Add(req.Amount)[m
[32m+[m	[32mcase WITHDRAW:[m
[32m+[m		[32mif balance.LessThan(req.Amount) {[m
[32m+[m			[32mreturn balance, fmt.Errorf("insufficient funds"), "Insufficient funds"[m
[32m+[m		[32m}[m
[32m+[m		[32mbalance = balance.Sub(req.Amount)[m
[32m+[m	[32m}[m
[32m+[m
[32m+[m	[32m_, err = tx.Exec(context.Background(), "UPDATE wallets SET balance=$1 WHERE wallet_id=$2", balance, req.WalletId)[m
[32m+[m	[32mif err != nil {[m
[32m+[m		[32mreturn decimal.Zero, err, "Failed to update balance"[m
[32m+[m	[32m}[m
[32m+[m
[32m+[m	[32merr = tx.Commit(context.Background())[m
[32m+[m	[32mif err != nil {[m
[32m+[m		[32mreturn decimal.Zero, err, "Transaction commit error"[m
[32m+[m	[32m}[m
[32m+[m
[32m+[m	[32mreturn balance, nil, ""[m
[32m+[m[32m}[m
[32m+[m
 func main() {[m
 	// Load configuration using viper[m
 	viper.SetConfigFile("config.env")[m
[36m@@ -64,9 +213,9 @@[m [mfunc main() {[m
 	r.POST("/api/v1/wallet", handleWalletOperation)[m
 	r.GET("/api/v1/wallets/:walletId", handleGetBalance)[m
 [m
[31m-	viper.SetDefault("HTTP_PORT", ":8080")[m
[32m+[m	[32mviper.SetDefault("HTTP_PORT", "8080")[m
 	httpPort := viper.GetString("HTTP_PORT")[m
[31m-	r.Run(httpPort)[m
[32m+[m	[32mr.Run(fmt.Sprintf(":%v", httpPort))[m
 }[m
 [m
 func createTablesIfNotExist() error {[m
[36m@@ -86,64 +235,45 @@[m [mfunc handleWalletOperation(c *gin.Context) {[m
 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})[m
 		return[m
 	}[m
[31m-[m
[31m-	tx, err := db.Begin(context.Background())[m
[32m+[m	[32m// Check if WalletId is uuid.UUID[m
[32m+[m	[32mwalletUUID, err := uuid.Parse(req.WalletId)[m
 	if err != nil {[m
[31m-		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction error"})[m
[32m+[m		[32mc.JSON(http.StatusBadRequest, gin.H{"error": "Invalid walletId format"})[m
 		return[m
 	}[m
[31m-	defer tx.Rollback(context.Background())[m
[31m-[m
[31m-	var balance decimal.Decimal[m
[31m-	err = tx.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1 FOR UPDATE", req.WalletId).Scan(&balance)[m
[31m-	if err != nil {[m
[31m-		if err.Error() == "no rows in result set" {[m
[31m-			// Create wallet if not exists[m
[31m-			balance = decimal.Zero[m
[31m-			_, err = tx.Exec(context.Background(), "INSERT INTO wallets (wallet_id, balance) VALUES ($1, $2)", req.WalletId, balance)[m
[31m-			if err != nil {[m
[31m-				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create wallet"})[m
[31m-				return[m
[31m-			}[m
[32m+[m	[32m// Push operation to queue for this wallet[m
[32m+[m	[32mch := getOrCreateQueue(walletUUID)[m
[32m+[m	[32mrespCh := make(chan opResult)[m
[32m+[m	[32mch <- &walletOpTask{ctx: c, req: req, resp: respCh}[m
[32m+[m	[32mres := <-respCh[m
[32m+[m	[32mif res.err != nil {[m
[32m+[m		[32mif res.msg == "Insufficient funds" {[m
[32m+[m			[32mc.JSON(http.StatusBadRequest, gin.H{"error": res.msg})[m
 		} else {[m
[31m-			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read balance"})[m
[31m-			return[m
[31m-		}[m
[31m-	}[m
[31m-[m
[31m-	switch req.OperationType {[m
[31m-	case DEPOSIT:[m
[31m-		balance = balance.Add(req.Amount)[m
[31m-	case WITHDRAW:[m
[31m-		if balance.LessThan(req.Amount) {[m
[31m-			c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient funds"})[m
[31m-			return[m
[32m+[m			[32mc.JSON(http.StatusInternalServerError, gin.H{"error": res.msg})[m
 		}[m
[31m-		balance = balance.Sub(req.Amount)[m
[32m+[m		[32mreturn[m
 	}[m
[32m+[m	[32mc.JSON(http.StatusOK, gin.H{"walletId": req.WalletId, "balance": res.balance})[m
[32m+[m[32m}[m
 [m
[31m-	_, err = tx.Exec(context.Background(), "UPDATE wallets SET balance=$1 WHERE wallet_id=$2", balance, req.WalletId)[m
[32m+[m[32mfunc handleGetBalance(c *gin.Context) {[m
[32m+[m	[32mwalletIdStr := c.Param("walletId")[m
[32m+[m	[32mwalletId, err := uuid.Parse(walletIdStr)[m
 	if err != nil {[m
[31m-		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update balance"})[m
[32m+[m		[32mc.JSON(http.StatusBadRequest, gin.H{"error": "Invalid walletId format"})[m
 		return[m
 	}[m
[31m-[m
[31m-	err = tx.Commit(context.Background())[m
[31m-	if err != nil {[m
[31m-		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction commit error"})[m
[32m+[m	[32mif balance, ok := GetBalanceCache(walletId); ok {[m
[32m+[m		[32mc.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance, "cached": true})[m
 		return[m
 	}[m
[31m-[m
[31m-	c.JSON(http.StatusOK, gin.H{"walletId": req.WalletId, "balance": balance})[m
[31m-}[m
[31m-[m
[31m-func handleGetBalance(c *gin.Context) {[m
[31m-	walletId := c.Param("walletId")[m
 	var balance decimal.Decimal[m
[31m-	err := db.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1", walletId).Scan(&balance)[m
[32m+[m	[32merr = db.QueryRow(context.Background(), "SELECT balance FROM wallets WHERE wallet_id=$1", walletId).Scan(&balance)[m
 	if err != nil {[m
 		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})[m
 		return[m
 	}[m
[31m-	c.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance})[m
[32m+[m	[32mSetBalanceCache(walletId, balance)[m
[32m+[m	[32mc.JSON(http.StatusOK, gin.H{"walletId": walletId, "balance": balance, "cached": false})[m
 }[m
[1mdiff --git a/go.mod b/go.mod[m
[1mindex c994da3..f3321ef 100644[m
[1m--- a/go.mod[m
[1m+++ b/go.mod[m
[36m@@ -6,6 +6,7 @@[m [mtoolchain go1.24.5[m
 [m
 require ([m
 	github.com/gin-gonic/gin v1.9.1[m
[32m+[m	[32mgithub.com/google/uuid v1.6.0[m
 	github.com/jackc/pgx/v5 v5.5.4[m
 	github.com/shopspring/decimal v1.3.1[m
 	github.com/spf13/viper v1.17.0[m
[1mdiff --git a/go.sum b/go.sum[m
[1mindex 18cf4f5..34924fe 100644[m
[1m--- a/go.sum[m
[1m+++ b/go.sum[m
[36m@@ -141,6 +141,8 @@[m [mgithub.com/google/pprof v0.0.0-20201203190320-1bf35d6f28c2/go.mod h1:kpwsk12EmLe[m
 github.com/google/pprof v0.0.0-20201218002935-b9804c9f04c2/go.mod h1:kpwsk12EmLew5upagYY7GY0pfYCcupk39gWOCRROcvE=[m
 github.com/google/renameio v0.1.0/go.mod h1:KWCgfxg9yswjAJkECMjeO8J8rahYeXnNhOm40UhjYkI=[m
 github.com/google/uuid v1.1.2/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=[m
[32m+[m[32mgithub.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=[m
[32m+[m[32mgithub.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=[m
 github.com/googleapis/gax-go/v2 v2.0.4/go.mod h1:0Wqv26UfaUD9n4G6kQubkQ+KchISgw+vpHVxEJEs9eg=[m
 github.com/googleapis/gax-go/v2 v2.0.5/go.mod h1:DWXyrwAJ9X0FpwwEdw+IPEYBICEFu5mhpdKc/us6bOk=[m
 github.com/googleapis/google-cloud-go-testing v0.0.0-20200911160855-bcd43fbb19e8/go.mod h1:dvDLG8qkwmyD9a/MJJN3XJcT3xFxOKAvTZGvuZmac9g=[m
