# Ledger Service

I am making the project to experiment with GO and postgres to create a fun ledger microservice.

# Key Features

- Data is immuatble
- Idempotent
- Concurrent Safe

# Types and Rules

## Account
- A unique "bank" account with a history of transaction entries.

## Entry
- Represents one side of a transaction and an account is an list of entries.

## Transaction
- Transactions must have at least two entries. Transaction are between two accounts, one recieveing and one sending.
- A tranansaction must never be zero
- A transaction must sum to zero.


# Milestones
- TBD