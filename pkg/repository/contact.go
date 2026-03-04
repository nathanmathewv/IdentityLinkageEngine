package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"identitylinkageengine/pkg/models"
)

// FetchContactGroup returns every contact related to the incoming email/phone:
// direct matches, their parent primaries, and siblings under those same primaries.
// FOR UPDATE locks the rows so concurrent requests block rather than race.
func FetchContactGroup(ctx context.Context, tx pgx.Tx, email, phone *string) ([]models.Contact, error) {
	query := `
		SELECT DISTINCT c.id, c.phone_number, c.email, c.linked_id,
		       c.link_precedence, c.created_at, c.updated_at, c.deleted_at
		FROM contact c
		WHERE c.deleted_at IS NULL AND (
			($1::text IS NOT NULL AND c.email = $1)
			OR ($2::text IS NOT NULL AND c.phone_number = $2)
			OR c.id IN (
				SELECT linked_id FROM contact
				WHERE deleted_at IS NULL AND linked_id IS NOT NULL
				AND (($1::text IS NOT NULL AND email = $1) OR ($2::text IS NOT NULL AND phone_number = $2))
			)
			OR c.linked_id IN (
				SELECT id FROM contact
				WHERE deleted_at IS NULL
				AND (($1::text IS NOT NULL AND email = $1) OR ($2::text IS NOT NULL AND phone_number = $2))
			)
		)
		ORDER BY c.created_at ASC
		FOR UPDATE`

	rows, err := tx.Query(ctx, query, email, phone)
	if err != nil {
		return nil, fmt.Errorf("fetching contact group: %w", err)
	}
	defer rows.Close()

	return scanRows(rows)
}

// CreateContact inserts a new row and returns it with the DB-generated fields.
func CreateContact(ctx context.Context, tx pgx.Tx, phone, email *string, linkedID *int, precedence models.LinkPrecedence) (models.Contact, error) {
	var c models.Contact
	err := tx.QueryRow(ctx, `
		INSERT INTO contact (phone_number, email, linked_id, link_precedence, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, phone_number, email, linked_id, link_precedence, created_at, updated_at, deleted_at`,
		phone, email, linkedID, precedence,
	).Scan(
		&c.ID, &c.PhoneNumber, &c.Email,
		&c.LinkedID, &c.LinkPrecedence,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	return c, err
}

// DemoteToSecondary flips a primary contact to secondary under the absolute primary.
func DemoteToSecondary(ctx context.Context, tx pgx.Tx, id, newLinkedID int) error {
	_, err := tx.Exec(ctx, `
		UPDATE contact
		SET link_precedence = 'secondary', linked_id = $2, updated_at = NOW()
		WHERE id = $1`,
		id, newLinkedID,
	)
	return err
}

// ReparentContact updates a secondary's linked_id to point at the absolute primary.
// Needed when that secondary's original parent just got demoted.
func ReparentContact(ctx context.Context, tx pgx.Tx, id, newLinkedID int) error {
	_, err := tx.Exec(ctx, `
		UPDATE contact SET linked_id = $2, updated_at = NOW() WHERE id = $1`,
		id, newLinkedID,
	)
	return err
}

// FetchAllUnderPrimary pulls the primary row and all its secondaries,
// ordered oldest-first so the primary is always at position 0.
func FetchAllUnderPrimary(ctx context.Context, tx pgx.Tx, primaryID int) ([]models.Contact, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, phone_number, email, linked_id, link_precedence, created_at, updated_at, deleted_at
		FROM contact
		WHERE deleted_at IS NULL AND (id = $1 OR linked_id = $1)
		ORDER BY created_at ASC`,
		primaryID,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching contacts under primary %d: %w", primaryID, err)
	}
	defer rows.Close()

	return scanRows(rows)
}

func scanRows(rows pgx.Rows) ([]models.Contact, error) {
	var contacts []models.Contact
	for rows.Next() {
		var c models.Contact
		if err := rows.Scan(
			&c.ID, &c.PhoneNumber, &c.Email,
			&c.LinkedID, &c.LinkPrecedence,
			&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
		); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}
