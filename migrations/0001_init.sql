-- 0001_init.sql
-- Creates the initial tables for the ledger

CREATE TABLE Accounts (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name    TEXT NOT NULL
);

CREATE TABLE Transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	idempotency_key TEXT NOT NULL UNIQUE,
	description     TEXT
);

CREATE TABLE Entries  (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL,
    FOREIGN KEY     (account_id)
    REFERENCES      Accounts(id),
    transaction_id   UUID NOT NULL,
    FOREIGN KEY     (transaction_id) 
    REFERENCES      Transactions(id),
    amount          BIGINT NOT NULL
);

