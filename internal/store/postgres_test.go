package store

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Casalis/LedgerFun/internal/ledger"
	"github.com/gofor-little/env"
)

func setup() (store *Store) {
	if err := env.Load("../../.env2"); err != nil {
		panic(err)
	}
	// var conn pgx.Conn
	// var err error
	conn, err := Connect()

	if err != nil {
		fmt.Printf("failed to connect to DB : %d", err)
		return nil
	}

	return New(conn)

}

// cleanDB removes all transactions and entries so tests don't observe
// state left over from previous runs. Accounts are left in place since
// MakeAccount looks them up by name and reuses them.
func cleanDB(s *Store) {
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, "DELETE FROM entries"); err != nil {
		panic(err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM transactions"); err != nil {
		panic(err)
	}
}

func MakeAccount(s *Store, accountName string) ledger.Account {
	var newAccount ledger.Account // Populate this structure with DB data
	newAccount, err := s.GetAccountByName(context.Background(), accountName)
	if err != nil {
		newAccount, err = s.CreateAccount(context.Background(), accountName)
		if err != nil {
			panic(err)
		}
	}

	return newAccount
}

func TestIdempotency(t *testing.T) {
	s := setup()
	if s == nil {
		t.Fatal("setup returned nil store")
	}
	t.Cleanup(func() { s.pool.Close() })
	cleanDB(s)
	t.Cleanup(func() { cleanDB(s) })

	var accNameA string = "Account A"
	var accNameB string = "Account B"

	accA := MakeAccount(s, accNameA)
	accB := MakeAccount(s, accNameB)

	// Create valid entries
	var entry1 ledger.Entry
	entry1.AccountID = accA.ID
	entry1.Amount = 100
	var entry2 ledger.Entry
	entry2.AccountID = accB.ID
	entry2.Amount = -100

	newTransaction, err := ledger.NewTransaction("Test Transaction", []ledger.Entry{entry1, entry2})

	ctx := context.Background()

	balanceABefore, err := s.GetBalance(ctx, accA.ID)
	if err != nil {
		t.Fatalf("failed to get initial balance for account A: %v", err)
	}
	balanceBBefore, err := s.GetBalance(ctx, accB.ID)
	if err != nil {
		t.Fatalf("failed to get initial balance for account B: %v", err)
	}

	// First post should succeed and move the balances.
	if _, err := s.PostTransaction(ctx, newTransaction); err != nil {
		t.Fatalf("first PostTransaction failed: %v", err)
	}

	balanceAAfterFirst, err := s.GetBalance(ctx, accA.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account A after first post: %v", err)
	}
	if balanceAAfterFirst != balanceABefore+entry1.Amount {
		t.Fatalf("account A balance = %d, want %d", balanceAAfterFirst, balanceABefore+entry1.Amount)
	}
	balanceBAfterFirst, err := s.GetBalance(ctx, accB.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account B after first post: %v", err)
	}
	if balanceBAfterFirst != balanceBBefore+entry2.Amount {
		t.Fatalf("account B balance = %d, want %d", balanceBAfterFirst, balanceBBefore+entry2.Amount)
	}

	// Re-posting the exact same transaction (same idempotency key, same body)
	// is a legitimate replay: it must succeed and must not move the
	// balances a second time.
	if _, err := s.PostTransaction(ctx, newTransaction); err != nil {
		t.Fatalf("replayed PostTransaction failed, want success: %v", err)
	}

	balanceAAfterReplay, err := s.GetBalance(ctx, accA.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account A after replay: %v", err)
	}
	if balanceAAfterReplay != balanceAAfterFirst {
		t.Fatalf("account A balance changed on replay: got %d, want %d", balanceAAfterReplay, balanceAAfterFirst)
	}
	balanceBAfterReplay, err := s.GetBalance(ctx, accB.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account B after replay: %v", err)
	}
	if balanceBAfterReplay != balanceBAfterFirst {
		t.Fatalf("account B balance changed on replay: got %d, want %d", balanceBAfterReplay, balanceBAfterFirst)
	}

	// Reusing the same key with a different body must be rejected.
	conflicting, err := ledger.NewTransaction("Different body", []ledger.Entry{entry1, entry2})
	if err != nil {
		t.Fatalf("failed to build conflicting transaction: %v", err)
	}
	conflicting.IdempotencyKey = newTransaction.IdempotencyKey

	if _, err := s.PostTransaction(ctx, conflicting); err == nil {
		t.Fatal("PostTransaction with reused key and different body succeeded, want error")
	}

	balanceAAfterConflict, err := s.GetBalance(ctx, accA.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account A after conflict: %v", err)
	}
	if balanceAAfterConflict != balanceAAfterFirst {
		t.Fatalf("account A balance changed on rejected conflict: got %d, want %d", balanceAAfterConflict, balanceAAfterFirst)
	}
	balanceBAfterConflict, err := s.GetBalance(ctx, accB.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account B after conflict: %v", err)
	}
	if balanceBAfterConflict != balanceBAfterFirst {
		t.Fatalf("account B balance changed on rejected conflict: got %d, want %d", balanceBAfterConflict, balanceBAfterFirst)
	}
}

func TestConcurrentPosting(t *testing.T) {
	// Testing concurrent insert into DB
	// Should be handled by Postgres by keeping entries append-only and deriving balance via SUM

	s := setup()
	if s == nil {
		t.Fatal("setup returned nil store")
	}
	t.Cleanup(func() { s.pool.Close() })
	cleanDB(s)
	t.Cleanup(func() { cleanDB(s) })

	var accNameA string = "Concurrent A"
	var accNameB string = "Concurrent B"

	ctx := context.Background()
	accA := MakeAccount(s, accNameA)
	accB := MakeAccount(s, accNameB)

	startA, _ := s.GetBalance(ctx, accA.ID)
	startB, _ := s.GetBalance(ctx, accB.ID)

	const n = 100
	const amount = 10

	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tx, err := ledger.NewTransaction(
				fmt.Sprintf("concurrent transfer %d", i),
				[]ledger.Entry{
					{AccountID: accA.ID, Amount: -amount},
					{AccountID: accB.ID, Amount: amount},
				},
			)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := s.PostTransaction(ctx, tx); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("PostTransaction failed: %v", err)
	}

	endA, err := s.GetBalance(ctx, accA.ID)
	if err != nil {
		t.Fatalf("GetBalance A: %v", err)
	}
	endB, err := s.GetBalance(ctx, accB.ID)
	if err != nil {
		t.Fatalf("GetBalance B: %v", err)
	}

	if endA != startA-n*amount {
		t.Errorf("account A balance = %d, want %d", endA, startA-n*amount)
	}
	if endB != startB+n*amount {
		t.Errorf("account B balance = %d, want %d", endB, startB+n*amount)
	}

	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COALESCE(SUM(amount), 0) FROM entries").Scan(&total); err != nil {
		t.Fatalf("sum entries: %v", err)
	}
	if total != 0 {
		t.Errorf("SUM(amount) over all entries = %d, want 0", total)
	}
}

func TestConcurrentIdempotentReplay(t *testing.T) {
	s := setup()
	if s == nil {
		t.Fatal("setup returned nil store")
	}
	t.Cleanup(func() { s.pool.Close() })
	cleanDB(s)
	t.Cleanup(func() { cleanDB(s) })

	ctx := context.Background()
	accA := MakeAccount(s, "Replay A")
	accB := MakeAccount(s, "Replay B")

	tx, err := ledger.NewTransaction("same key, concurrent", []ledger.Entry{
		{AccountID: accA.ID, Amount: -50},
		{AccountID: accB.ID, Amount: 50},
	})
	if err != nil {
		t.Fatalf("build transaction: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.PostTransaction(ctx, tx); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("PostTransaction (same key, concurrent) failed: %v", err)
	}

	var count int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM transactions WHERE idempotency_key = $1", tx.IdempotencyKey).Scan(&count); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if count != 1 {
		t.Errorf("transactions with idempotency_key = %d, want 1", count)
	}

	balA, _ := s.GetBalance(ctx, accA.ID)
	if balA != -50 {
		t.Errorf("account A balance = %d, want -50 (transaction should apply exactly once)", balA)
	}
}
