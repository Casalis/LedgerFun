package ledger

import "errors"

// Unique id
type uuid string

type Account struct {
	ID   uuid
	Name string
}

// Represents a single side of a transaction
type Entry struct {
	AccountID uuid
	Amount    int64
}

// Represents a transaction
type Transaction struct {
	ID             uuid
	IdempotencyKey string
	Description    string
	Entries        []Entry
}

var (
	ErrTooFewEntries = errors.New("transaction must have at least 2 entries")
	ErrZeroEntry     = errors.New("entry amount must not be zero")
	ErrUnbalanced    = errors.New("entries must sum to zero")
)

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
