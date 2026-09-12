package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type GameStats struct {
	RomPath   string
	RomName   string
	System    string
	TotalSecs int
	Plays     int
	AvgSecs   int
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	conn, err := sql.Open("sqlite", filepath.Join(dir, "playtime.db"))
	if err != nil {
		return nil, err
	}
	// Single writer, keep it simple
	conn.SetMaxOpenConns(1)
	s := &Store{db: conn}
	return s, s.migrate()
}

func (s *Store) Close() { s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			system     TEXT    NOT NULL,
			rom_path   TEXT    NOT NULL,
			rom_name   TEXT    NOT NULL,
			started_at INTEGER NOT NULL,
			ended_at   INTEGER,
			duration   INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_rom ON sessions(rom_path);
	`)
	return err
}

func (s *Store) StartSession(system, romPath, romName string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (system, rom_path, rom_name, started_at) VALUES (?, ?, ?, ?)`,
		system, romPath, romName, time.Now().Unix(),
	)
	return err
}

func (s *Store) EndSession(romPath string) error {
	now := time.Now().Unix()
	// Update the most recent open session for this rom
	_, err := s.db.Exec(`
		UPDATE sessions SET ended_at = ?, duration = ? - started_at
		WHERE id = (
			SELECT id FROM sessions
			WHERE rom_path = ? AND ended_at IS NULL
			ORDER BY started_at DESC LIMIT 1
		)
	`, now, now, romPath)
	return err
}

// TopGames returns all played games sorted by total play time descending.
// Sessions under minDuration seconds are ignored (filters accidental launches).
func (s *Store) TopGames(minDuration int) ([]GameStats, error) {
	return s.query(`
		SELECT rom_path, rom_name, system,
		       CAST(SUM(duration) AS INTEGER)     AS total_secs,
		       COUNT(*)                           AS plays,
		       CAST(AVG(duration) AS INTEGER)     AS avg_secs
		FROM sessions
		WHERE duration >= ?
		GROUP BY rom_path
		ORDER BY total_secs DESC
	`, minDuration)
}

// TopGamesBySystem is TopGames filtered to one system.
func (s *Store) TopGamesBySystem(system string, minDuration int) ([]GameStats, error) {
	return s.query(`
		SELECT rom_path, rom_name, system,
		       CAST(SUM(duration) AS INTEGER)     AS total_secs,
		       COUNT(*)                           AS plays,
		       CAST(AVG(duration) AS INTEGER)     AS avg_secs
		FROM sessions
		WHERE duration >= ? AND system = ?
		GROUP BY rom_path
		ORDER BY total_secs DESC
	`, minDuration, system)
}

func (s *Store) query(sql string, args ...any) ([]GameStats, error) {
	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var games []GameStats
	for rows.Next() {
		var g GameStats
		if err := rows.Scan(&g.RomPath, &g.RomName, &g.System, &g.TotalSecs, &g.Plays, &g.AvgSecs); err != nil {
			continue
		}
		games = append(games, g)
	}
	return games, rows.Err()
}

// Systems returns systems that have recorded play data.
func (s *Store) Systems(minDuration int) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT system FROM sessions WHERE duration >= ? ORDER BY system`,
		minDuration,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sys string
		rows.Scan(&sys)
		out = append(out, sys)
	}
	return out, rows.Err()
}

// TotalSecs returns the grand total of all recorded play time.
func (s *Store) TotalSecs(minDuration int) (int, error) {
	var total int
	err := s.db.QueryRow(
		`SELECT COALESCE(SUM(duration), 0) FROM sessions WHERE duration >= ?`,
		minDuration,
	).Scan(&total)
	return total, err
}
