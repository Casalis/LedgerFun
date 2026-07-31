package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Casalis/LedgerFun/internal/ledger"
	"github.com/Casalis/LedgerFun/internal/store"
	"github.com/gofor-little/env"
	"github.com/google/uuid"
)

/*
* TESTS REQUIRE THE DATABASE TO BE RUNNING
 */

func setup(t *testing.T) *Handler {
	t.Helper()

	if err := env.Load("../../.env2"); err != nil {
		t.Fatalf("failed to load .env2: %v", err)
	}

	conn, err := store.Connect()
	if err != nil {
		t.Fatalf("failed to connect to DB: %v", err)
	}
	t.Cleanup(conn.Close)

	return NewHandler(store.New(conn))
}

// doRequest sends a request straight into the handler's mux, bypassing the
// network - this is a table-stakes way to test net/http handlers without
// spinning up a real listener.
func doRequest(h *Handler, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

// decode unmarshals a recorder's body, failing the test on bad JSON.
func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("failed to decode response body %q: %v", rec.Body.String(), err)
	}
	return v
}

// uniqueName gives each test its own account so tests don't need to share
// or reset database state between runs.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s %s", prefix, uuid.NewString())
}

func TestHealthz(t *testing.T) {
	h := setup(t)

	rec := doRequest(h, http.MethodGet, "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestCreateAccount(t *testing.T) {
	h := setup(t)

	name := uniqueName("Account")
	rec := doRequest(h, http.MethodPost, "/v1/accounts", map[string]string{"name": name})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	got := decode[ledger.Account](t, rec)
	if got.Name != name {
		t.Fatalf("account name = %q, want %q", got.Name, name)
	}
	if got.ID == uuid.Nil {
		t.Fatal("account id was not populated")
	}
}

func TestCreateAccount_MissingName(t *testing.T) {
	h := setup(t)

	rec := doRequest(h, http.MethodPost, "/v1/accounts", map[string]string{"name": ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	h := setup(t)

	rec := doRequest(h, http.MethodGet, "/v1/accounts/"+uuid.NewString(), nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestGetAccount_InvalidID(t *testing.T) {
	h := setup(t)

	rec := doRequest(h, http.MethodGet, "/v1/accounts/not-a-uuid", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// createAccount is a small helper for tests that need a real account to
// transact against.
func createAccount(t *testing.T, h *Handler, name string) ledger.Account {
	t.Helper()
	rec := doRequest(h, http.MethodPost, "/v1/accounts", map[string]string{"name": name})
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create account %q: status = %d, body = %s", name, rec.Code, rec.Body.String())
	}
	return decode[ledger.Account](t, rec)
}

func getBalance(t *testing.T, h *Handler, accountID uuid.UUID) int64 {
	t.Helper()
	rec := doRequest(h, http.MethodGet, "/v1/accounts/"+accountID.String()+"/balance", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to get balance: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode balance response: %v", err)
	}
	return resp.Balance
}

func TestPostTransaction_MovesBalances(t *testing.T) {
	h := setup(t)

	accA := createAccount(t, h, uniqueName("Account A"))
	accB := createAccount(t, h, uniqueName("Account B"))

	beforeA := getBalance(t, h, accA.ID)
	beforeB := getBalance(t, h, accB.ID)

	body := map[string]any{
		"description": "test transfer",
		"entries": []map[string]any{
			{"account_id": accA.ID, "amount": -500},
			{"account_id": accB.ID, "amount": 500},
		},
	}
	rec := doRequest(h, http.MethodPost, "/v1/transactions", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	tx := decode[ledger.Transaction](t, rec)

	if got := getBalance(t, h, accA.ID); got != beforeA-500 {
		t.Fatalf("account A balance = %d, want %d", got, beforeA-500)
	}
	if got := getBalance(t, h, accB.ID); got != beforeB+500 {
		t.Fatalf("account B balance = %d, want %d", got, beforeB+500)
	}

	// GetTransaction should return exactly what was posted.
	getRec := doRequest(h, http.MethodGet, "/v1/transactions/"+tx.ID.String(), nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetTransaction status = %d, want %d, body = %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	fetched := decode[ledger.Transaction](t, getRec)
	if fetched.ID != tx.ID || len(fetched.Entries) != 2 {
		t.Fatalf("fetched transaction = %+v, want to match posted transaction %+v", fetched, tx)
	}
}

func TestPostTransaction_Unbalanced(t *testing.T) {
	h := setup(t)

	accA := createAccount(t, h, uniqueName("Account A"))
	accB := createAccount(t, h, uniqueName("Account B"))

	body := map[string]any{
		"description": "bad transfer",
		"entries": []map[string]any{
			{"account_id": accA.ID, "amount": -500},
			{"account_id": accB.ID, "amount": 400},
		},
	}
	rec := doRequest(h, http.MethodPost, "/v1/transactions", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPostTransaction_ReplayIsIdempotent(t *testing.T) {
	h := setup(t)

	accA := createAccount(t, h, uniqueName("Account A"))
	accB := createAccount(t, h, uniqueName("Account B"))
	key := uuid.NewString()

	body := map[string]any{
		"description":     "replay test",
		"idempotency_key": key,
		"entries": []map[string]any{
			{"account_id": accA.ID, "amount": -200},
			{"account_id": accB.ID, "amount": 200},
		},
	}

	first := doRequest(h, http.MethodPost, "/v1/transactions", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first post status = %d, want %d, body = %s", first.Code, http.StatusCreated, first.Body.String())
	}
	firstTx := decode[ledger.Transaction](t, first)

	balanceAfterFirst := getBalance(t, h, accA.ID)

	second := doRequest(h, http.MethodPost, "/v1/transactions", body)
	if second.Code != http.StatusCreated {
		t.Fatalf("replay status = %d, want %d, body = %s", second.Code, http.StatusCreated, second.Body.String())
	}
	secondTx := decode[ledger.Transaction](t, second)

	if secondTx.ID != firstTx.ID {
		t.Fatalf("replay returned a different transaction id: got %s, want %s", secondTx.ID, firstTx.ID)
	}
	if got := getBalance(t, h, accA.ID); got != balanceAfterFirst {
		t.Fatalf("balance changed on replay: got %d, want %d", got, balanceAfterFirst)
	}
}

func TestPostTransaction_KeyReuseConflict(t *testing.T) {
	h := setup(t)

	accA := createAccount(t, h, uniqueName("Account A"))
	accB := createAccount(t, h, uniqueName("Account B"))
	key := uuid.NewString()

	first := doRequest(h, http.MethodPost, "/v1/transactions", map[string]any{
		"description":     "original",
		"idempotency_key": key,
		"entries": []map[string]any{
			{"account_id": accA.ID, "amount": -200},
			{"account_id": accB.ID, "amount": 200},
		},
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("first post status = %d, want %d, body = %s", first.Code, http.StatusCreated, first.Body.String())
	}

	conflicting := doRequest(h, http.MethodPost, "/v1/transactions", map[string]any{
		"description":     "different body, same key",
		"idempotency_key": key,
		"entries": []map[string]any{
			{"account_id": accA.ID, "amount": -1},
			{"account_id": accB.ID, "amount": 1},
		},
	})

	if conflicting.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", conflicting.Code, http.StatusConflict, conflicting.Body.String())
	}
}

func TestListEntries(t *testing.T) {
	h := setup(t)

	accA := createAccount(t, h, uniqueName("Account A"))
	accB := createAccount(t, h, uniqueName("Account B"))

	doRequest(h, http.MethodPost, "/v1/transactions", map[string]any{
		"description": "entry listing",
		"entries": []map[string]any{
			{"account_id": accA.ID, "amount": -50},
			{"account_id": accB.ID, "amount": 50},
		},
	})

	rec := doRequest(h, http.MethodGet, "/v1/accounts/"+accA.ID.String()+"/entries", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	entries := decode[[]ledger.Entry](t, rec)
	if len(entries) != 1 || entries[0].Amount != -50 {
		t.Fatalf("entries = %+v, want a single -50 entry", entries)
	}
}
