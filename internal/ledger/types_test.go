package ledger

import (
	"testing"

	"github.com/google/uuid"
)

func TestTransaction(t *testing.T) {
	var uuid1 uuid.UUID = uuid.New()
	var uuid2 uuid.UUID = uuid.New()

	tests := []struct {
		entries []Entry
		wantErr error
	}{
		{
			entries: []Entry{
				{AccountID: uuid1, Amount: -100},
				{AccountID: uuid2, Amount: 100},
			},
			wantErr: nil,
		},
		{
			entries: []Entry{
				{AccountID: uuid1, Amount: -100},
			},
			wantErr: ErrTooFewEntries,
		},
		{
			entries: []Entry{
				{AccountID: uuid1, Amount: -100},
				{AccountID: uuid2, Amount: 0},
			},
			wantErr: ErrZeroEntry,
		},
		{
			entries: []Entry{
				{AccountID: uuid1, Amount: -100},
				{AccountID: uuid2, Amount: 50},
			},
			wantErr: ErrUnbalanced,
		},
	}

	for _, tc := range tests {
		newTransaction := Transaction{Entries: tc.entries}
		err := newTransaction.Validate()

		if err != tc.wantErr {
			t.Errorf("Transaction Validate Failed - Expected: %d Actual: %d", tc.wantErr, err)
		}
	}
}
