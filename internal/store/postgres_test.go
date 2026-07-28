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

func teardown() {
	//
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

	var accNameA string = "Account A"
	var accNameB string = "Account B"

	accA := MakeAccount(s, accNameA)
	accB := MakeAccount(s, accNameB)

	// Need to create a create transaction func
	var newTransaction ledger.Transaction
	newTransaction.Description = "Bills"
	// Create valid entries
	var entry1 ledger.Entry
	entry1.AccountID = accA.ID
	entry1.Amount = 100
	var entry2 ledger.Entry
	entry2.AccountID = accB.ID
	entry2.Amount = -100
	newTransaction.Entries = []ledger.Entry{entry1, entry2}

	var err error
	newTransaction.IdempotencyKey, err = newTransaction.GetHash()
	if err != nil {
		panic(err)
	}

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

	// Re-posting the identical transaction hashes to the same idempotency key
	// and must be rejected, without moving the balances a second time.
	if _, err := s.PostTransaction(ctx, newTransaction); err == nil {
		t.Fatal("second PostTransaction with reused idempotency key succeeded, want error")
	}

	balanceAAfterSecond, err := s.GetBalance(ctx, accA.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account A after second post: %v", err)
	}
	if balanceAAfterSecond != balanceAAfterFirst {
		t.Fatalf("account A balance changed on rejected repost: got %d, want %d", balanceAAfterSecond, balanceAAfterFirst)
	}
	balanceBAfterSecond, err := s.GetBalance(ctx, accB.ID)
	if err != nil {
		t.Fatalf("failed to get balance for account B after second post: %v", err)
	}
	if balanceBAfterSecond != balanceBAfterFirst {
		t.Fatalf("account B balance changed on rejected repost: got %d, want %d", balanceBAfterSecond, balanceBAfterFirst)
	}
}
