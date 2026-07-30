package ledger

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
)

type Account struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Represents a single side of a transaction
type Entry struct {
	AccountID uuid.UUID `json:"account_id"`
	Amount    int64     `json:"amount"`
}

// Represents a transaction
type Transaction struct {
	ID             uuid.UUID `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Description    string    `json:"description"`
	Entries        []Entry   `json:"entries"`
}

var (
	ErrTooFewEntries = errors.New("transaction must have at least 2 entries")
	ErrZeroEntry     = errors.New("entry amount must not be zero")
	ErrUnbalanced    = errors.New("entries must sum to zero")
)

func NewTransaction(description string, entries []Entry) (Transaction, error) {
	var t Transaction
	t.Description = description
	t.IdempotencyKey = uuid.NewString()
	t.Entries = entries
	return t, t.Validate()
}

func (t Transaction) Validate() error {
	// Validate entry count
	if len(t.Entries) < 2 {
		return ErrTooFewEntries
	}

	// Validate entry sum
	var sum int64 = 0
	for _, e := range t.Entries {
		if e.Amount == 0 {
			return ErrZeroEntry
		}

		sum += e.Amount
	}
	if sum != 0 {
		return ErrUnbalanced
	}

	return nil
}

func (t Transaction) ToString() string {
	return ""
}

func (t Transaction) GetHash() (string, error) {

	// Sort a copy of the entries so hash equality doesn't depend on the
	// order rows come back from the database (row order isn't guaranteed
	// to match insertion order since ids are random UUIDs).
	entries := slices.Clone(t.Entries)
	slices.SortFunc(entries, func(a, b Entry) int {
		if c := cmp.Compare(a.AccountID.String(), b.AccountID.String()); c != 0 {
			return c
		}
		return cmp.Compare(a.Amount, b.Amount)
	})

	// Serialise only the caller-supplied fields - ID is assigned by the
	// store on insert, so including it would make a genuine replay of the
	// same request hash differently from the stored copy.
	data, err := json.Marshal(struct {
		IdempotencyKey string
		Description    string
		Entries        []Entry
	}{t.IdempotencyKey, t.Description, entries})
	if err != nil {
		return "", err
	}
	// Compute hash
	hash := sha256.Sum256(data)

	// Get Hex string
	return hex.EncodeToString(hash[:]), nil

}
