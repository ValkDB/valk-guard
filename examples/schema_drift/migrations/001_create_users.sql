-- Migration 001: create users table
CREATE TABLE users (
    id    SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    name  TEXT
);
