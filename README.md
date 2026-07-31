# Ledger Fun

A double-entry ledger service, written in Go against Postgres, built as an evenings-and-weekends
project to actually *learn* Go rather than read about it. I'd used Go a little before, but wanted
something meaty enough to hit real problems - database transactions, concurrency, idempotency -
instead of another todo app.

Why a ledger? Because "money must always balance" is a genuinely fun invariant to enforce in code,
and it's a small enough domain that I can take it seriously: correct on paper, then correct under
concurrent load, then correct over the network.

## The core idea: double-entry

Every transaction is a set of entries that must sum to exactly zero. Money doesn't appear or
vanish, it only ever moves from one account to another. If `Validate()` on a `Transaction`
returns `nil`, the books balance by construction, not by convention.

```go
tx, err := ledger.NewTransaction("Coffee", []ledger.Entry{
    {AccountID: alice, Amount: -350},
    {AccountID: cafe, Amount: 350},
})
```

## What's actually built so far

This is in progress and I'd rather the README say so honestly than oversell it.

- [x] **Domain model** - `Account`, `Entry`, `Transaction`, with validation for entry count,
      zero-amount entries, and balance (`internal/ledger`)
- [x] **Postgres store** - accounts, transactions and entries persisted via `pgx`, with real
      DB-transaction semantics (`internal/store`)
- [x] **Idempotency** - every transaction carries a key; replaying the same request returns the
      original result with no side effects, while reusing a key with a *different* body is
      rejected. This turned out to be the most interesting bug-hunt in the project so far, see
      below.
- [x] **HTTP API** - a thin `net/http` layer over the store (`internal/api`), see below
- [ ] Concurrency safety under load (row locking, proven with a goroutine stress test)
- [ ] Logging, metrics, deploy

## Future stretch plans
- **Web UI / Game mode** - Create web interface to visualise, create and manage `Accounts`, define who the 
    account represents and an AI starts perfoming transactions with a pool of money.

## The idempotency bug I liked most

Idempotency is one of those things that's easy to describe and easy to get subtly wrong. My first
pass looked right (same key twice, second call rejected, test green) but the test was passing
for the wrong reason. The hash comparison used to detect "is this genuinely the same request"
included the database assigned transaction ID, which by definition never matches between the
client's copy and the stored copy. So *every* replay looked like a conflict, not just real ones.
Fixed by hashing only the caller-supplied fields, and sorting entries before hashing so that row
order coming back from Postgres (which isn't guaranteed to match insertion order) can't change the
result. A good reminder that a green test tells you your assertions passed, not that they were the
right assertions.

## The API

A small `net/http` layer (Go 1.22+ pattern routing, no framework) sits over the store. Handlers
decode JSON, call the store, encode JSON back - the validation and idempotency logic all lives one
layer down, so the HTTP layer stays thin.

| Method | Path                       | Does                                    |
|--------|----------------------------|------------------------------------------|
| GET    | `/healthz`                 | liveness check                           |
| POST   | `/v1/accounts`              | create an account                        |
| GET    | `/v1/accounts/{id}`         | fetch an account                         |
| GET    | `/v1/accounts/{id}/balance` | current balance                          |
| GET    | `/v1/accounts/{id}/entries` | entries posted against an account        |
| POST   | `/v1/transactions`          | post a (validated, idempotent) transaction |
| GET    | `/v1/transactions/{id}`     | fetch a transaction and its entries      |

```bash
alice=$(curl -s -X POST localhost:8080/v1/accounts -d '{"name":"Alice"}' | jq -r .id)
bob=$(curl -s -X POST localhost:8080/v1/accounts -d '{"name":"Bob"}'   | jq -r .id)

curl -s -X POST localhost:8080/v1/transactions -d '{
  "description": "Coffee",
  "entries": [
    {"account_id": "'"$alice"'", "amount": -350},
    {"account_id": "'"$bob"'",   "amount": 350}
  ]
}'

curl -s localhost:8080/v1/accounts/$alice/balance
```

Errors are plain JSON (`{"error": "..."}`) with the status code doing the real work: `400` for
invalid input, `404` for a missing account/transaction, `409` for reusing an idempotency key with
a different body.

## Running it locally

```bash
docker run --name ledger-db -e POSTGRES_PASSWORD=devpass -p 5432:5432 -d postgres:16
psql -h localhost -U postgres -f migrations/0001_init.sql

# .env2
CONNECTION_STRING=postgres://postgres:devpass@localhost:5432

go run ./cmd/ledgerd
go test ./...
```

Tests in `internal/store` and `internal/api` run against a real Postgres instance rather than a
mock - the interesting bugs in this project (see below) were the kind that only show up against
the real thing.

## What's next

A concurrency test next: many goroutines posting against the same two accounts, proving the
balance stays exact under a race (and adding row locking when it inevitably doesn't, at first).
After that: logging, containerising, and a small deployed instance to link here.

## Design notes for the curious

- **Whyo not soft-delete or edit transacti0ns?** Ledgers are append-only. Mistakes get corrected
  with a reversing entry, never a mutation - that's what makes the balance invariant trustworthy.
- **Why hash-based idempotency under the hood?** The store accepts a caller-supplied key, but
  compares by content hash internally, so "same key, same body" (a safe replay) and "same key,
  different body" (a real conflict) are distinguishable rather than just trusting the key alone.
