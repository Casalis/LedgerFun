package store

import (
	"context"
	"fmt"

	"github.com/Casalis/LedgerFun/internal/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateAccount(ctx context.Context, name string) (ledger.Account, error) {
	row := s.pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ($1) RETURNING id, name", name)
	var a ledger.Account
	err := row.Scan(&a.ID, &a.Name)
	if err != nil {
		fmt.Print("CreateAccount failed.")
	}
	return a, err
}

func (s *Store) GetAccount(ctx context.Context, id uuid.UUID) (ledger.Account, error) {
	row := s.pool.QueryRow(ctx, "SELECT * FROM accounts WHERE id = $1", id)

	var a ledger.Account
	err := row.Scan(&a.ID, &a.Name)
	if err != nil {
		fmt.Print("GetAccount failed.")
	}
	return a, err
}

func (s *Store) GetAccountByName(ctx context.Context, name string) (ledger.Account, error) {
	row := s.pool.QueryRow(ctx, "SELECT * FROM accounts WHERE name = $1", name)

	var a ledger.Account
	err := row.Scan(&a.ID, &a.Name)
	if err != nil {
		fmt.Print("GetAccount failed.")
	}
	return a, err
}

func (s *Store) PostTransaction(ctx context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
	// This transaction will be populated with the succesfully submited data
	var t ledger.Transaction

	err := tx.Validate()
	if err != nil {
		return t, err
	}

	dtx, err := s.pool.Begin(ctx)
	if err != nil {
		return t, err
	}

	// Build the transaction
	row := dtx.QueryRow(ctx, "INSERT INTO transactions (idempotency_key, description) VALUES ($1, $2) RETURNING id", tx.IdempotencyKey, tx.Description)
	// Verify transaction init
	err = row.Scan(&t.ID)
	if err == nil {
		for _, e := range tx.Entries {
			row := dtx.QueryRow(ctx, "INSERT INTO entries (account_id, transaction_id, amount) VALUES ($1, $2, $3) RETURNING account_id, amount", e.AccountID, t.ID, e.Amount)

			var _e ledger.Entry
			err = row.Scan(&_e.AccountID, &_e.Amount)
			if err != nil {
				break
			}

			t.Entries = append(t.Entries, _e)
		}
	}

	if err == nil {
		err = dtx.Commit(ctx)
	} else {
		err2 := dtx.Rollback(ctx)
		fmt.Print(err2)
	}

	return t, err
}

func (s *Store) GetBalance(ctx context.Context, accountId uuid.UUID) (int64, error) {
	r, err := s.pool.Query(ctx, "SELECT amount FROM entries WHERE account_id = $1 ", accountId)
	if err != nil {
		return 0, err
	}
	var sum int64 = 0
	var value int64
	_, err = pgx.ForEachRow(r, []any{&value}, func() error {
		sum += int64(value)
		return nil
	})

	return sum, err

}
