DROP TABLE visitor_hashes;

CREATE TABLE visitor_days (
	hash TEXT NOT NULL,
	host TEXT NOT NULL,
	year INTEGER NOT NULL,
	year_day INTEGER NOT NULL,
	first_seen DATETIME,
	PRIMARY KEY (hash, host, year, year_day)
);

ALTER TABLE hourly_stats DROP COLUMN unique_visitors;
