-- Dangerous queries that sneak into production code

SELECT * FROM users;

SELECT * FROM orders;

UPDATE users SET active = false;

DELETE FROM sessions;

SELECT id FROM users WHERE email LIKE '%@gmail.com';

DROP TABLE archived_users;

CREATE INDEX idx_users_email ON users(email);
