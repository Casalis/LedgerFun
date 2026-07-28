package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type Account struct {
	ID   uuid.UUID
	Name string
}

// Represents a single side of a transaction
type Entry struct {
	AccountID uuid.UUID
	Amount    int64
}

// Represents a transaction
type Transaction struct {
	ID             uuid.UUID
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

func (t Transaction) ToString() string {
	return ""
}

func (t Transaction) GetHash() (string, error) {

	// Serialise
	data, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	// Compute hash
	hash := sha256.Sum256(data)

	// Get Hex string
	return hex.EncodeToString(hash[:]), nil

}
