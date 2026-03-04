package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"identitylinkageengine/pkg/models"
	"identitylinkageengine/pkg/repository"
)

// seedRequest mirrors the Contact shape from the spec so you can POST exact
// fixtures — including custom IDs, timestamps, and linkage — to set up test
// scenarios without relying on the sequence or wall clock.
type seedRequest struct {
	ID             int                     `json:"id"`
	PhoneNumber    *string                 `json:"phoneNumber"`
	Email          *string                 `json:"email"`
	LinkedID       *int                    `json:"linkedId"`
	LinkPrecedence models.LinkPrecedence   `json:"linkPrecedence"`
	CreatedAt      time.Time               `json:"createdAt"`
	UpdatedAt      time.Time               `json:"updatedAt"`
	DeletedAt      *time.Time              `json:"deletedAt"`
}

func Seed(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req seedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.LinkPrecedence != models.Primary && req.LinkPrecedence != models.Secondary {
			writeError(w, http.StatusBadRequest, "linkPrecedence must be 'primary' or 'secondary'")
			return
		}

		c := models.Contact{
			ID:             req.ID,
			PhoneNumber:    req.PhoneNumber,
			Email:          req.Email,
			LinkedID:       req.LinkedID,
			LinkPrecedence: req.LinkPrecedence,
			CreatedAt:      req.CreatedAt,
			UpdatedAt:      req.UpdatedAt,
			DeletedAt:      req.DeletedAt,
		}

		created, err := repository.SeedContact(r.Context(), pool, c)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	}
}
