SELECT id, name FROM users WHERE active = true;

INSERT INTO logs (user_id, action) VALUES (1, 'login');

SELECT count(*) FROM orders WHERE created_at > '2024-01-01';
