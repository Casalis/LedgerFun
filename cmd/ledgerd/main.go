package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/Casalis/LedgerFun/internal/ledger"
	"github.com/Casalis/LedgerFun/internal/store"
	"github.com/gofor-little/env"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEnvVarNotSet = errors.New("required environment variables not set")
)

func main() {

	if err := env.Load(".env2"); err != nil {
		panic(err)
	}

	// var conn pgx.Conn
	// var err error
	conn, err := connect()

	if err != nil {
		fmt.Printf("failed to connect to DB : %d", err)
		return
	}

	var accNameA string = "Account A"
	var accNameB string = "Account B"

	s := store.New(conn)

	var accA ledger.Account // Populate this structure with DB data
	accA, err = s.GetAccountByName(context.Background(), accNameA)
	if err != nil {
		accA, err = s.CreateAccount(context.Background(), accNameA)
		if err != nil {
			panic(err)
		}
	}

	var accB ledger.Account // Populate this structure with DB data
	accB, err = s.GetAccountByName(context.Background(), accNameB)
	if err != nil {
		accB, err = s.CreateAccount(context.Background(), accNameB)
		if err != nil {
			panic(err)
		}
	}

	// Need to create a create transaction func
	var newTransaction ledger.Transaction
	newTransaction.Description = "Bills"
	newTransaction.IdempotencyKey = uuid.NewString()
	// Create valid entries
	var entry1 ledger.Entry
	entry1.AccountID = accA.ID
	entry1.Amount = 100
	var entry2 ledger.Entry
	entry2.AccountID = accB.ID
	entry2.Amount = -100
	// Add entries to transaction
	newTransaction.Entries = append(newTransaction.Entries, entry1, entry2)

	_, err = s.PostTransaction(context.Background(), newTransaction)

	balance, err := s.GetBalance(context.Background(), accB.ID)
	fmt.Print(balance, err)
	if err == nil {
		fmt.Print("YAY")

	}

	conn.Close()
}

func connect() (*pgxpool.Pool, error) {
	connString := env.Get("CONNECTION_STRING", "NOT_SET")

	if connString == "NOT_SET" {
		return nil, ErrEnvVarNotSet
	}
	fmt.Printf("Hello world")
	conn, err := pgxpool.New(context.Background(), connString)

	if err != nil {
		return nil, err
	}
	return conn, nil
}
