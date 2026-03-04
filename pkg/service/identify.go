package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"identitylinkageengine/pkg/dsu"
	"identitylinkageengine/pkg/models"
	"identitylinkageengine/pkg/repository"
)

type IdentifyResponse struct {
	Contact ContactPayload `json:"contact"`
}

type ContactPayload struct {
	PrimaryContactID    int      `json:"primaryContactId"`
	Emails              []string `json:"emails"`
	PhoneNumbers        []string `json:"phoneNumbers"`
	SecondaryContactIDs []int    `json:"secondaryContactIds"`
}

func Identify(ctx context.Context, pool *pgxpool.Pool, email, phone *string) (*IdentifyResponse, error) {
	if email == nil && phone == nil {
		return nil, errors.New("at least one of email or phoneNumber is required")
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	matches, err := repository.FetchContactGroup(ctx, tx, email, phone)
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		created, err := repository.CreateContact(ctx, tx, phone, email, nil, models.Primary)
		if err != nil {
			return nil, fmt.Errorf("creating primary contact: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		return buildResponse(created.ID, []models.Contact{created}), nil
	}

	// Build the request-scoped DSU from whatever rows we fetched.
	// Rows are ordered by created_at ASC, so the first contact registered into
	// emailMap/phoneMap is always the oldest one claiming that identifier.
	d := dsu.New()
	for _, c := range matches {
		d.Add(c.ID, c.LinkedID, c.Email, c.PhoneNumber, c.CreatedAt)
	}

	// If both fields came in, union whatever roots own them.
	// This is the core merge step — two previously separate groups become one.
	if email != nil && phone != nil {
		emailOwner, emailFound := d.EmailMap[*email]
		phoneOwner, phoneFound := d.PhoneMap[*phone]
		if emailFound && phoneFound {
			d.Union(emailOwner, phoneOwner)
		}
	}

	// After union, all contacts in the set resolve to the same root.
	absolutePrimaryID := d.Find(matches[0].ID)

	// Write back the structural changes the union produced.
	for _, c := range matches {
		if c.ID == absolutePrimaryID {
			continue
		}
		switch {
		case c.LinkPrecedence == models.Primary:
			// Lost the union — was an independent primary before this request linked it.
			if err := repository.DemoteToSecondary(ctx, tx, c.ID, absolutePrimaryID); err != nil {
				return nil, fmt.Errorf("demoting contact %d: %w", c.ID, err)
			}
		case c.LinkedID != nil && *c.LinkedID != absolutePrimaryID:
			// Secondary whose parent just got demoted — reparent straight to absolute primary
			// so we never have secondary -> secondary chains in the DB.
			if err := repository.ReparentContact(ctx, tx, c.ID, absolutePrimaryID); err != nil {
				return nil, fmt.Errorf("reparenting contact %d: %w", c.ID, err)
			}
		}
	}

	// If the request carries a field value we have not seen before, record it as
	// a new secondary so future lookups by that value land in the right group.
	existingEmails := map[string]bool{}
	existingPhones := map[string]bool{}
	for _, c := range matches {
		if c.Email != nil {
			existingEmails[*c.Email] = true
		}
		if c.PhoneNumber != nil {
			existingPhones[*c.PhoneNumber] = true
		}
	}

	newEmail := email != nil && !existingEmails[*email]
	newPhone := phone != nil && !existingPhones[*phone]

	if newEmail || newPhone {
		if _, err := repository.CreateContact(ctx, tx, phone, email, &absolutePrimaryID, models.Secondary); err != nil {
			return nil, fmt.Errorf("creating secondary contact: %w", err)
		}
	}

	all, err := repository.FetchAllUnderPrimary(ctx, tx, absolutePrimaryID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return buildResponse(absolutePrimaryID, all), nil
}

func buildResponse(primaryID int, contacts []models.Contact) *IdentifyResponse {
	emails := []string{}
	phones := []string{}
	secondaryIDs := []int{}

	emailSeen := map[string]bool{}
	phoneSeen := map[string]bool{}

	// Primary contact fields go first per spec.
	for _, c := range contacts {
		if c.ID != primaryID {
			continue
		}
		if c.Email != nil && !emailSeen[*c.Email] {
			emails = append(emails, *c.Email)
			emailSeen[*c.Email] = true
		}
		if c.PhoneNumber != nil && !phoneSeen[*c.PhoneNumber] {
			phones = append(phones, *c.PhoneNumber)
			phoneSeen[*c.PhoneNumber] = true
		}
		break
	}

	for _, c := range contacts {
		if c.ID == primaryID {
			continue
		}
		secondaryIDs = append(secondaryIDs, c.ID)
		if c.Email != nil && !emailSeen[*c.Email] {
			emails = append(emails, *c.Email)
			emailSeen[*c.Email] = true
		}
		if c.PhoneNumber != nil && !phoneSeen[*c.PhoneNumber] {
			phones = append(phones, *c.PhoneNumber)
			phoneSeen[*c.PhoneNumber] = true
		}
	}

	return &IdentifyResponse{
		Contact: ContactPayload{
			PrimaryContactID:    primaryID,
			Emails:              emails,
			PhoneNumbers:        phones,
			SecondaryContactIDs: secondaryIDs,
		},
	}
}
