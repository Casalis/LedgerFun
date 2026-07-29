package store

import (
	"context"
	"fmt"
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
