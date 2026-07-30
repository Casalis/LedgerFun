package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Casalis/LedgerFun/internal/ledger"
	"github.com/Casalis/LedgerFun/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	store *store.Store
}

func NewHandler(s *store.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.Healthz)

	mux.HandleFunc("POST /v1/accounts", h.CreateAccount)
	mux.HandleFunc("GET /v1/accounts/{id}", h.GetAccount)
	mux.HandleFunc("GET /v1/accounts/{id}/balance", h.GetBalance)
	mux.HandleFunc("GET /v1/accounts/{id}/entries", h.ListEntries)

	mux.HandleFunc("POST /v1/transactions", h.PostTransaction)
	mux.HandleFunc("GET /v1/transactions/{id}", h.GetTransaction)

	return mux
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type createAccountRequest struct {
	Name string `json:"name"`
}

func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	account, err := h.store.CreateAccount(r.Context(), req.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, account)
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	account, err := h.store.GetAccount(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, account)
}

type balanceResponse struct {
	AccountID uuid.UUID `json:"account_id"`
	Balance   int64     `json:"balance"`
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	// Confirm the account exists before reporting a (potentially zero) balance for it.
	if _, err := h.store.GetAccount(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}

	balance, err := h.store.GetBalance(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, balanceResponse{AccountID: id, Balance: balance})
}

func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	if _, err := h.store.GetAccount(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}

	entries, err := h.store.ListEntries(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

type postTransactionRequest struct {
	Description    string         `json:"description"`
	IdempotencyKey string         `json:"idempotency_key"`
	Entries        []ledger.Entry `json:"entries"`
}

func (h *Handler) PostTransaction(w http.ResponseWriter, r *http.Request) {
	var req postTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var tx ledger.Transaction
	var err error
	if req.IdempotencyKey == "" {
		tx, err = ledger.NewTransaction(req.Description, req.Entries)
	} else {
		tx = ledger.Transaction{
			IdempotencyKey: req.IdempotencyKey,
			Description:    req.Description,
			Entries:        req.Entries,
		}
		err = tx.Validate()
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	posted, err := h.store.PostTransaction(r.Context(), tx)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, posted)
}

func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transaction id")
		return
	}

	tx, err := h.store.GetTransaction(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, tx)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeStoreError maps an error coming out of the store/domain layer to an
// HTTP status code and writes it as a JSON error body.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrKeyReuse):
		writeError(w, http.StatusConflict, "idempotency key reused with a different transaction body")
	case errors.Is(err, ledger.ErrTooFewEntries),
		errors.Is(err, ledger.ErrZeroEntry),
		errors.Is(err, ledger.ErrUnbalanced):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
