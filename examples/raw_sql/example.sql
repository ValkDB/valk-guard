-- Examples of SQL Anti-Patterns detected by Valk Guard

-- VG001: Select Star
SELECT * FROM users;

-- VG004: Unbounded Select (No LIMIT)
SELECT id, email FROM users;

-- VG002: Update without WHERE
UPDATE users SET active = false;

-- VG005: LIKE with leading wildcard
SELECT id FROM users WHERE email LIKE '%@gmail.com';

-- Valid Query (Passes all checks)
SELECT id, email FROM users WHERE active = true LIMIT 100;

-- Inline Disable Example
-- valk-guard:disable VG001
SELECT * FROM logs;
