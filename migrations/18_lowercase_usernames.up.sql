-- Usernames are unique case-insensitively by storing them in lowercase. If two
-- usernames differ only in case, the unique index makes this fail loudly
-- instead of silently merging accounts.
UPDATE users SET username = lower(username) WHERE username <> lower(username);
