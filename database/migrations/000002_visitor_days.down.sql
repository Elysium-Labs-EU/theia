ALTER TABLE hourly_stats ADD COLUMN unique_visitors INTEGER DEFAULT 0;

DROP TABLE visitor_days;

CREATE TABLE visitor_hashes (
	hash TEXT PRIMARY KEY,
	hour_bucket INTEGER,
	first_seen DATETIME
);
