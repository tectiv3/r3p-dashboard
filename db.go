package main

import (
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type DB struct {
	writer *sqlx.DB
	reader *sqlx.DB
}

type Reading struct {
	TS    int64  `json:"ts" db:"ts"`
	Topic string `json:"topic" db:"topic"`
	Value int64  `json:"value" db:"value"`
}

type AggReading struct {
	TS    int64  `json:"ts" db:"ts"`
	Topic string `json:"topic" db:"topic"`
	Min   int64  `json:"min" db:"min"`
	Max   int64  `json:"max" db:"max"`
	Avg   int64  `json:"avg" db:"avg"`
}

type Event struct {
	TS      int64  `json:"ts" db:"ts"`
	Type    string `json:"type" db:"type"`
	Payload string `json:"payload" db:"payload"`
}

const schema = `
CREATE TABLE IF NOT EXISTS readings (
    ts    INTEGER NOT NULL,
    topic TEXT    NOT NULL,
    value INTEGER NOT NULL,
    PRIMARY KEY (ts, topic)
);

CREATE TABLE IF NOT EXISTS readings_5m (
    ts    INTEGER NOT NULL,
    topic TEXT    NOT NULL,
    min   INTEGER NOT NULL,
    max   INTEGER NOT NULL,
    avg   INTEGER NOT NULL,
    PRIMARY KEY (ts, topic)
);

CREATE TABLE IF NOT EXISTS readings_1h (
    ts    INTEGER NOT NULL,
    topic TEXT    NOT NULL,
    min   INTEGER NOT NULL,
    max   INTEGER NOT NULL,
    avg   INTEGER NOT NULL,
    PRIMARY KEY (ts, topic)
);

CREATE TABLE IF NOT EXISTS readings_1d (
    ts    INTEGER NOT NULL,
    topic TEXT    NOT NULL,
    min   INTEGER NOT NULL,
    max   INTEGER NOT NULL,
    avg   INTEGER NOT NULL,
    PRIMARY KEY (ts, topic)
);

CREATE TABLE IF NOT EXISTS events (
    ts      INTEGER NOT NULL,
    type    TEXT    NOT NULL,
    payload TEXT    NOT NULL,
    PRIMARY KEY (ts, type)
);

CREATE INDEX IF NOT EXISTS idx_readings_topic_ts ON readings(topic, ts);
`

func OpenDB(path string) (*DB, error) {
	dsn := path + "?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)"

	writer, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	if _, err := writer.Exec(schema); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	reader, err := sqlx.Open("sqlite", dsn+"&_pragma=query_only(1)")
	if err != nil {
		return nil, fmt.Errorf("open reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	return &DB{writer: writer, reader: reader}, nil
}

func (d *DB) Close() error {
	d.reader.Close()
	return d.writer.Close()
}

func (d *DB) InsertReading(ts int64, topic string, value int64) error {
	_, err := d.writer.Exec(
		"INSERT OR REPLACE INTO readings (ts, topic, value) VALUES (?, ?, ?)",
		ts, topic, value,
	)
	return err
}

func (d *DB) InsertEvent(ts int64, typ string, payload string) error {
	_, err := d.writer.Exec(
		"INSERT OR REPLACE INTO events (ts, type, payload) VALUES (?, ?, ?)",
		ts, typ, payload,
	)
	return err
}

func (d *DB) QueryLatest() ([]Reading, error) {
	rows := make([]Reading, 0)
	err := d.reader.Select(&rows, `
		SELECT r.ts, r.topic, r.value
		FROM readings r
		INNER JOIN (
			SELECT topic, MAX(ts) as max_ts FROM readings GROUP BY topic
		) latest ON r.topic = latest.topic AND r.ts = latest.max_ts
	`)
	return rows, err
}

// resolution picks the preferred pre-aggregated table and bucket size.
// Returns ("", 0) for raw data (no downsampling needed).
func resolution(from, to int64) (table string, bucket int64) {
	span := to - from
	switch {
	case span <= 6*3600:
		return "", 0
	case span <= 7*86400:
		return "readings_5m", 300
	case span <= 90*86400:
		return "readings_1h", 3600
	default:
		return "readings_1d", 86400
	}
}

type HistoryPoint struct {
	TS    int64 `json:"ts" db:"ts"`
	Value int64 `json:"value" db:"value"`
	Min   int64 `json:"min" db:"min"`
	Max   int64 `json:"max" db:"max"`
}

func (d *DB) QueryHistory(topic string, from, to int64) ([]HistoryPoint, error) {
	aggTable, bucket := resolution(from, to)

	// Short range: return raw readings
	if aggTable == "" {
		rows := make([]HistoryPoint, 0)
		err := d.reader.Select(&rows, `
			SELECT ts, value, value AS min, value AS max
			FROM readings WHERE topic = ? AND ts >= ? AND ts <= ?
			ORDER BY ts`, topic, from, to)
		return rows, err
	}

	// Try pre-aggregated table first
	rows := make([]HistoryPoint, 0)
	err := d.reader.Select(&rows, fmt.Sprintf(`
		SELECT ts, avg AS value, min, max
		FROM %s WHERE topic = ? AND ts >= ? AND ts <= ?
		ORDER BY ts`, aggTable), topic, from, to)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}

	// Aggregated table empty — downsample from raw readings on the fly
	err = d.reader.Select(&rows, fmt.Sprintf(`
		SELECT (ts / %d) * %d AS ts,
		       CAST(AVG(value) AS INTEGER) AS value,
		       MIN(value) AS min,
		       MAX(value) AS max
		FROM readings WHERE topic = ? AND ts >= ? AND ts <= ?
		GROUP BY (ts / %d) * %d
		ORDER BY ts`, bucket, bucket, bucket, bucket), topic, from, to)
	return rows, err
}

func (d *DB) QueryEvents(from, to int64) ([]Event, error) {
	rows := make([]Event, 0)
	err := d.reader.Select(&rows, `
		SELECT ts, type, payload FROM events
		WHERE ts >= ? AND ts <= ? ORDER BY ts`,
		from, to,
	)
	return rows, err
}

func (d *DB) Compact() error {
	now := time.Now().Unix()
	tiers := []struct {
		source, target string
		threshold      int64
		bucket         int64
	}{
		{"readings", "readings_5m", 7 * 86400, 300},
		{"readings_5m", "readings_1h", 90 * 86400, 3600},
		{"readings_1h", "readings_1d", 2 * 365 * 86400, 86400},
	}

	for _, t := range tiers {
		cutoff := now - t.threshold
		if err := d.compactTier(t.source, t.target, cutoff, t.bucket); err != nil {
			return fmt.Errorf("compact %s→%s: %w", t.source, t.target, err)
		}
	}
	return nil
}

func (d *DB) compactTier(source, target string, cutoff, bucket int64) error {
	tx, err := d.writer.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if source == "readings" {
		_, err = tx.Exec(fmt.Sprintf(`
			INSERT OR REPLACE INTO %s (ts, topic, min, max, avg)
			SELECT (ts / %d) * %d, topic, MIN(value), MAX(value), AVG(value)
			FROM %s
			WHERE ts < ?
			GROUP BY (ts / %d) * %d, topic`,
			target, bucket, bucket, source, bucket, bucket),
			cutoff,
		)
	} else {
		_, err = tx.Exec(fmt.Sprintf(`
			INSERT OR REPLACE INTO %s (ts, topic, min, max, avg)
			SELECT (ts / %d) * %d, topic, MIN(min), MAX(max), AVG(avg)
			FROM %s
			WHERE ts < ?
			GROUP BY (ts / %d) * %d, topic`,
			target, bucket, bucket, source, bucket, bucket),
			cutoff,
		)
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE ts < ?", source), cutoff)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("Compacted %s → %s (cutoff %d)", source, target, cutoff)
	return nil
}
