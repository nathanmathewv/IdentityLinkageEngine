package models

import "time"

type LinkPrecedence string

const (
	Primary   LinkPrecedence = "primary"
	Secondary LinkPrecedence = "secondary"
)

type Contact struct {
	ID             int
	PhoneNumber    *string
	Email          *string
	LinkedID       *int
	LinkPrecedence LinkPrecedence
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
