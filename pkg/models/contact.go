package models

import "time"

type LinkPrecedence string

const (
	Primary   LinkPrecedence = "primary"
	Secondary LinkPrecedence = "secondary"
)

type Contact struct {
	ID             int            `json:"id"`
	PhoneNumber    *string        `json:"phoneNumber"`
	Email          *string        `json:"email"`
	LinkedID       *int           `json:"linkedId"`
	LinkPrecedence LinkPrecedence `json:"linkPrecedence"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt"`
}
