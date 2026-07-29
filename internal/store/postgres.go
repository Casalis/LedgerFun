package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/Casalis/LedgerFun/internal/ledger"
	"github.com/gofor-little/env"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEnvVarNotSet = errors.New("required environment variables not set")
	ErrKeyReuse     = errors.New("Transaction kley reused")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func Connect() (*pgxpool.Pool, error) {
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

	// Check for unique idempotency violation
	var pgErr *pgconn.PgError
	if err != nil && errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		dtx.Rollback(ctx)
		return s.HandleIdempotency(ctx, tx)
	}

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

func (s *Store) HandleIdempotency(ctx context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
	// We need to determine what type of idempotency problem this is

	tr := s.pool.QueryRow(ctx, "SELECT * FROM transactions WHERE idempotency_key = $1", tx.IdempotencyKey)
	var te ledger.Transaction
	err := tr.Scan(&te.ID, &te.IdempotencyKey, &te.Description)

	if err != nil {
		return te, err
	}

	r, err := s.pool.Query(ctx, "SELECT account_id, amount FROM entries WHERE transaction_id = $1 ORDER BY id", te.ID)
	if err != nil {
		return te, err
	}
	var entries []ledger.Entry
	var ea uuid.UUID
	var am int64
	_, err = pgx.ForEachRow(r, []any{&ea, &am}, func() error {
		var newEntry ledger.Entry
		newEntry.AccountID = ea
		newEntry.Amount = am
		entries = append(entries, newEntry)
		return nil
	})
	if err != nil {
		return te, err
	}
	te.Entries = entries

	txH, err := tx.GetHash()
	if err != nil {
		return tx, err
	}

	teH, err := te.GetHash()
	if err != nil {
		return te, err
	}

	if txH == teH {
		return te, nil
	}
	if tx.IdempotencyKey == te.IdempotencyKey && txH != teH {
		return te, ErrKeyReuse
	}

	return te, err
}
