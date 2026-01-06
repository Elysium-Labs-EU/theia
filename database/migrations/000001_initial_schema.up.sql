CREATE TABLE visitor_hashes (
	hash TEXT PRIMARY KEY,
	hour_bucket INTEGER,
	first_seen DATETIME
);

CREATE TABLE hourly_stats (
	hour INTEGER,
	year_day INTEGER,
	year INTEGER,
	path TEXT,
	host TEXT,
	page_views INTEGER DEFAULT 0,
	is_static INTEGER DEFAULT 0,
	unique_visitors INTEGER DEFAULT 0,
	bot_views INTEGER DEFAULT 0,
	PRIMARY KEY (hour, year_day, year, path, host)
);

CREATE TABLE hourly_status_codes (
	hour INTEGER,
	year_day INTEGER,
	year INTEGER,
	path TEXT,
	host TEXT,
	status_code INTEGER,
	count INTEGER DEFAULT 0,
	PRIMARY KEY (hour, year_day, year, path, host, status_code)
);

CREATE TABLE hourly_referrers (
	hour INTEGER,
	year_day INTEGER,
	year INTEGER,
	path TEXT,
	host TEXT,
	referrer TEXT,
	count INTEGER DEFAULT 0,
	PRIMARY KEY (hour, year_day, year, path, host, referrer)
);