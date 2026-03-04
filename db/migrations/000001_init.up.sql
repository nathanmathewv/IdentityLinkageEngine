DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'link_precedence') THEN
        CREATE TYPE link_precedence AS ENUM ('primary', 'secondary');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS contact (
    id              SERIAL PRIMARY KEY,
    phone_number    VARCHAR(20),
    email           VARCHAR(255),
    linked_id       INT REFERENCES contact(id),
    link_precedence link_precedence NOT NULL DEFAULT 'primary',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

-- partial indexes keep lookups fast without indexing NULL rows
CREATE INDEX IF NOT EXISTS idx_contact_email  ON contact(email)        WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contact_phone  ON contact(phone_number) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contact_linked ON contact(linked_id)    WHERE deleted_at IS NULL;
