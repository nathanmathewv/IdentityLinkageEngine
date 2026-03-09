package dsu_test

import (
	"fmt"
	"testing"
	"time"

	"identitylinkageengine/pkg/dsu"
)

type contact struct {
	id    int
	email string
	phone string
	ts    time.Time
}

func makeContacts(n int) []contact {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]contact, n)
	for i := 0; i < n; i++ {
		out[i] = contact{
			id:    i + 1,
			email: fmt.Sprintf("user%d@example.com", i+1),
			phone: fmt.Sprintf("9%09d", i+1),
			ts:    base.Add(time.Duration(i) * time.Second),
		}
	}
	return out
}

func identifyWithDSU(contacts []contact, inEmail, inPhone string) int {
	d := dsu.New()
	for _, c := range contacts {
		e, p := c.email, c.phone
		d.Add(c.id, nil, &e, &p, c.ts)
	}
	eOwner, eOK := d.EmailMap[inEmail]
	pOwner, pOK := d.PhoneMap[inPhone]
	if eOK && pOK {
		d.Union(eOwner, pOwner)
	}
	if eOK {
		return d.Find(eOwner)
	}
	if pOK {
		return d.Find(pOwner)
	}
	return -1
}

func identifyNaive(contacts []contact, inEmail, inPhone string) int {
	groupEmails := map[string]bool{inEmail: true}
	groupPhones := map[string]bool{inPhone: true}
	groupIDs := map[int]bool{}
	changed := true
	for changed {
		changed = false
		for _, c := range contacts {
			if groupEmails[c.email] || groupPhones[c.phone] {
				if !groupIDs[c.id] {
					groupIDs[c.id] = true
					groupEmails[c.email] = true
					groupPhones[c.phone] = true
					changed = true
				}
			}
		}
	}
	var primaryID int
	var primaryTS time.Time
	first := true
	for _, c := range contacts {
		if groupIDs[c.id] {
			if first || c.ts.Before(primaryTS) {
				primaryID = c.id
				primaryTS = c.ts
				first = false
			}
		}
	}
	return primaryID
}

func BenchmarkDSU_Small(b *testing.B) {
	contacts := makeContacts(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identifyWithDSU(contacts, contacts[3].email, contacts[7].phone)
	}
}

func BenchmarkNaive_Small(b *testing.B) {
	contacts := makeContacts(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identifyNaive(contacts, contacts[3].email, contacts[7].phone)
	}
}

func BenchmarkDSU_Medium(b *testing.B) {
	contacts := makeContacts(200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identifyWithDSU(contacts, contacts[50].email, contacts[150].phone)
	}
}

func BenchmarkNaive_Medium(b *testing.B) {
	contacts := makeContacts(200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identifyNaive(contacts, contacts[50].email, contacts[150].phone)
	}
}

func BenchmarkDSU_Large(b *testing.B) {
	contacts := makeContacts(2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identifyWithDSU(contacts, contacts[500].email, contacts[1500].phone)
	}
}

func BenchmarkNaive_Large(b *testing.B) {
	contacts := makeContacts(2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identifyNaive(contacts, contacts[500].email, contacts[1500].phone)
	}
}
