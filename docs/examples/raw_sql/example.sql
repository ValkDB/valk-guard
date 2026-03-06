-- Raw SQL examples for VG001-VG008.

-- VG001: select-star
SELECT * FROM users;

-- VG002: missing-where-update
UPDATE users SET active = false;

-- VG003: missing-where-delete
DELETE FROM sessions;

-- VG004: unbounded-select
SELECT id, email FROM users;

-- VG005: like-leading-wildcard
SELECT id FROM users WHERE email LIKE '%@gmail.com';

-- VG006: select-for-update-no-where
SELECT * FROM accounts FOR UPDATE;

-- VG007: destructive-ddl
DROP TABLE archived_users;

-- VG008: non-concurrent-index
CREATE INDEX idx_users_email ON users(email);

-- Valid query (should not trigger any rule)
SELECT id, email FROM users WHERE active = true LIMIT 100;

-- Inline disable example
-- valk-guard:disable VG001
SELECT * FROM logs;
