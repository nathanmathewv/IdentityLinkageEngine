package dsu

import "time"

// DSU is built fresh per /identify request from the rows fetched out of Postgres.
// It lives only for the duration of one transaction; the database is the real
// source of truth, this is just a computation helper.
type DSU struct {
	parent    map[int]int
	createdAt map[int]time.Time

	// EmailMap and PhoneMap mirror the df[email]=accountIndex trick from the
	// accounts-merge pattern. They give us O(1) access to "which contact owns
	// this email/phone?" so we can immediately locate which two roots to union.
	EmailMap map[string]int
	PhoneMap map[string]int
}

func New() *DSU {
	return &DSU{
		parent:    make(map[int]int),
		createdAt: make(map[int]time.Time),
		EmailMap:  make(map[string]int),
		PhoneMap:  make(map[string]int),
	}
}

// Add registers a contact into the DSU.
// Primaries are their own parent; secondaries point at their linkedID.
func (d *DSU) Add(id int, linkedID *int, email, phone *string, createdAt time.Time) {
	if linkedID != nil {
		d.parent[id] = *linkedID
	} else {
		d.parent[id] = id
	}
	d.createdAt[id] = createdAt

	// First-seen wins. Since we ORDER BY created_at ASC when fetching, the
	// oldest contact claiming a given email/phone is registered here.
	if email != nil && *email != "" {
		if _, exists := d.EmailMap[*email]; !exists {
			d.EmailMap[*email] = id
		}
	}
	if phone != nil && *phone != "" {
		if _, exists := d.PhoneMap[*phone]; !exists {
			d.PhoneMap[*phone] = id
		}
	}
}

// Find returns the root of x with path compression.
// Our DB enforces depth-1 chains (secondary always points directly to a primary),
// so this rarely recurses more than once — but path compression keeps it correct
// for any in-memory unions we perform before writing back.
func (d *DSU) Find(x int) int {
	if d.parent[x] != x {
		d.parent[x] = d.Find(d.parent[x])
	}
	return d.parent[x]
}

// Union merges the sets of a and b. The contact with the older createdAt becomes
// the root (the Absolute Primary). Ties broken by lower ID for stability.
func (d *DSU) Union(a, b int) {
	ra, rb := d.Find(a), d.Find(b)
	if ra == rb {
		return
	}

	aWins := d.createdAt[ra].Before(d.createdAt[rb]) ||
		(d.createdAt[ra].Equal(d.createdAt[rb]) && ra < rb)

	if aWins {
		d.parent[rb] = ra
	} else {
		d.parent[ra] = rb
	}
}
