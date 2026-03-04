# IdentityLinkageEngine

A backend service that reconciles customer identities across multiple contacts.

When a customer places orders using different email addresses and phone numbers, websites end up with fragmented contact records. This service receives any combination of `email` and `phoneNumber`, finds all related contacts, links them under a single primary, and returns the unified identity.

**Live:** https://identity-engine.onrender.com

---

## Table of Contents

1. [How it works](#how-it-works)
2. [DSU — the core algorithm](#dsu--the-core-algorithm)
3. [API reference](#api-reference)
4. [Database schema](#database-schema)
5. [Running locally](#running-locally)
6. [Docker](#docker)
7. [Deployment (Render.com)](#deployment-rendercom)

---

## How it works

Every contact is either **primary** (the oldest known record for a group) or **secondary** (a later record pointing at the primary via `linked_id`).

When `POST /identify` receives a request:

1. All contacts matching the supplied email or phone are fetched from Postgres inside a `SELECT FOR UPDATE` transaction.
2. A request-scoped DSU is built from those rows to compute the correct group structure in memory.
3. If the email and phone belong to two previously separate groups, the DSU unions them — the older primary survives, the newer one is demoted to secondary.
4. Any secondary pointing at the demoted primary is reparented directly to the absolute primary (no secondary→secondary chains in the DB).
5. If the request contains a new email or phone not yet seen, a new secondary is created.
6. The full consolidated group is returned.

The database is always the source of truth. The DSU is built fresh per request and discarded after the transaction commits.

---

## DSU — the core algorithm

The DSU (Disjoint Set Union / Union-Find) lives in `pkg/dsu/dsu.go`. It is built once per request from the rows fetched out of Postgres, runs entirely in memory, and is thrown away after the transaction.

```
parent    map[int]int         — each contact ID maps to its root
createdAt map[int]time.Time   — used to decide which root survives a union
EmailMap  map[string]int      — email  → contact ID that owns it (O(1) lookup)
PhoneMap  map[string]int      — phone  → contact ID that owns it (O(1) lookup)
```

**Add(id, linkedID, email, phone, createdAt)**
Registers a contact. Primaries are their own parent; secondaries point at their `linkedID`. Rows are fetched `ORDER BY created_at ASC`, so the first contact registered for a given email/phone in `EmailMap`/`PhoneMap` is always the oldest one — first-seen wins.

**Find(x) → root**
Standard path-compressed Find. The DB enforces depth-1 chains (secondaries always point directly at a primary), so this rarely recurses more than once.

**Union(a, b)**
Merges the sets of two contacts. The root with the **older** `createdAt` becomes the absolute primary. Ties broken by lower ID for stability.

The `EmailMap`/`PhoneMap` dictionaries let the service immediately answer "which root currently owns this email?" and "which root currently owns this phone?" without scanning every contact in the group — this is the same pattern as the classic *Accounts Merge* problem.

---

## API reference

### `POST /identify`

Receive a contact identifier and return the unified identity group.

**Request**
```json
{
  "email": "mcfly@hillvalley.edu",
  "phoneNumber": "123456"
}
```
At least one of `email` or `phoneNumber` must be present.

**Response `200`**
```json
{
  "contact": {
    "primaryContactId": 1,
    "emails": ["mcfly@hillvalley.edu"],
    "phoneNumbers": ["123456"],
    "secondaryContactIds": []
  }
}
```
`emails[0]` and `phoneNumbers[0]` are always the primary contact's values. Subsequent entries come from secondaries.

---

### `POST /contacts`  *(seed / testing)*

Insert a contact with explicit field values including a custom ID and timestamps. Useful for reproducing exact spec fixtures.

**Request**
```json
{
  "id": 11,
  "email": "george@hillvalley.edu",
  "phoneNumber": "919191",
  "linkPrecedence": "primary",
  "createdAt": "2023-04-01T00:00:00.000Z",
  "updatedAt": "2023-04-01T00:00:00.000Z"
}
```

**Response `201`** — the created contact object.

> After seeding contacts with custom IDs, reset the sequence so auto-increment doesn't collide:
> ```sql
> SELECT setval('contact_id_seq', (SELECT MAX(id) FROM contact));
> ```

---

### `GET /health`

Returns `200 ok`. Used by Render for liveness checks.

---

## Database schema

```sql
CREATE TYPE link_precedence AS ENUM ('primary', 'secondary');

CREATE TABLE contact (
    id              SERIAL PRIMARY KEY,
    phone_number    VARCHAR(20),
    email           VARCHAR(255),
    linked_id       INT REFERENCES contact(id),
    link_precedence link_precedence NOT NULL DEFAULT 'primary',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

-- partial indexes: only index non-deleted rows
CREATE INDEX idx_contact_email  ON contact(email)        WHERE deleted_at IS NULL;
CREATE INDEX idx_contact_phone  ON contact(phone_number) WHERE deleted_at IS NULL;
CREATE INDEX idx_contact_linked ON contact(linked_id)    WHERE deleted_at IS NULL;
```

Migrations run automatically on server startup via `golang-migrate`. Migration files are in `db/migrations/`.

---

## Running locally

**Prerequisites:** Go 1.25+, PostgreSQL 14+

```bash
# 1. Clone
git clone https://github.com/nathanmathewv/IdentityLinkageEngine
cd IdentityLinkageEngine

# 2. Create a Postgres database
createdb identitydb

# 3. Configure environment
cp .env.example .env
# Edit .env with your DB credentials

# 4. Run (migrations apply automatically on startup)
go run ./cmd/server/main.go
```

The server starts on the port defined by `APP_PORT` (default `8080`).

**Environment variables**

| Variable | Description | Default |
|---|---|---|
| `APP_PORT` | HTTP listen port | `8080` |
| `DB_HOST` | Postgres host | `localhost` |
| `DB_PORT` | Postgres port | `5432` |
| `DB_USER` | Postgres user | — |
| `DB_PASSWORD` | Postgres password | — |
| `DB_NAME` | Database name | — |
| `DB_SSLMODE` | SSL mode | `disable` |

**Quick test**

```bash
curl -s -X POST http://localhost:8080/identify \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","phoneNumber":"999"}' | jq
```

---

## Docker

Build and run the full stack (app + Postgres) with Docker Compose:

```bash
cp .env.example .env
# Fill in DB_USER, DB_PASSWORD, DB_NAME, APP_PORT in .env

docker compose -f deployments/docker/docker-compose.yml up --build
```

The app waits for Postgres to pass its healthcheck before starting. Migrations run automatically.

**Build the image alone**

```bash
docker build -f deployments/docker/Dockerfile -t identity-engine .
```

The Dockerfile is a two-stage build — `golang:1.25-alpine` compiles the binary, `alpine:3.19` runs it. The final image is ~20 MB.

---

## Deployment (Render.com)

The repo includes a `render.yaml` blueprint that provisions everything in one click.
