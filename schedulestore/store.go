package schedulestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("schedulestore: schedule not found")
	validID     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
)

// Store persists all schedules in one SQLite database.
type Store struct {
	path     string
	database *sql.DB
}

// New opens or creates a schedule database at path.
func New(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("schedulestore: path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("schedulestore: resolve path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return nil, fmt.Errorf("schedulestore: create parent directory: %w", err)
	}
	database, err := sql.Open("sqlite", absolute)
	if err != nil {
		return nil, fmt.Errorf("schedulestore: open SQLite: %w", err)
	}
	database.SetMaxOpenConns(1)
	store := &Store{path: absolute, database: database}
	if err := store.initialize(context.Background()); err != nil {
		database.Close()
		return nil, err
	}
	if err := os.Chmod(absolute, 0o600); err != nil {
		database.Close()
		return nil, fmt.Errorf("schedulestore: secure database: %w", err)
	}
	return store, nil
}

func (s *Store) initialize(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS schedules (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL DEFAULT '',
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER NOT NULL,
    data BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS schedules_conversation_id ON schedules(conversation_id);
CREATE INDEX IF NOT EXISTS schedules_created_at ON schedules(created_at_ns, id);`
	if _, err := s.database.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("schedulestore: initialize SQLite schema: %w", err)
	}
	return nil
}

func (s *Store) Path() string { return s.path }
func (s *Store) Close() error { return s.database.Close() }

// Create inserts schedule and supplies its creation and update timestamps.
func (s *Store) Create(ctx context.Context, schedule Schedule) (Schedule, error) {
	if err := validateID(schedule.ID); err != nil {
		return Schedule{}, err
	}
	now := time.Now().UTC()
	if schedule.CreatedAt.IsZero() {
		schedule.CreatedAt = now
	} else {
		schedule.CreatedAt = schedule.CreatedAt.UTC()
	}
	schedule.UpdatedAt = now
	normalizeOptionalTimes(&schedule)
	data, err := json.Marshal(schedule)
	if err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: encode schedule: %w", err)
	}
	_, err = s.database.ExecContext(ctx, `
INSERT INTO schedules(id, conversation_id, created_at_ns, updated_at_ns, data)
VALUES(?, ?, ?, ?, ?)`,
		schedule.ID, schedule.ConversationID, schedule.CreatedAt.UnixNano(), schedule.UpdatedAt.UnixNano(), data)
	if err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: create schedule %q: %w", schedule.ID, err)
	}
	return schedule, nil
}

func (s *Store) Get(ctx context.Context, id string) (Schedule, error) {
	if err := validateID(id); err != nil {
		return Schedule{}, err
	}
	var data []byte
	if err := s.database.QueryRowContext(ctx, `SELECT data FROM schedules WHERE id = ?`, id).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Schedule{}, ErrNotFound
		}
		return Schedule{}, fmt.Errorf("schedulestore: get schedule %q: %w", id, err)
	}
	return decodeSchedule(id, data)
}

func (s *Store) List(ctx context.Context) ([]Schedule, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT id, data FROM schedules ORDER BY created_at_ns, id`)
	if err != nil {
		return nil, fmt.Errorf("schedulestore: list schedules: %w", err)
	}
	defer rows.Close()
	var schedules []Schedule
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("schedulestore: scan schedule: %w", err)
		}
		schedule, err := decodeSchedule(id, data)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("schedulestore: list schedules: %w", err)
	}
	return schedules, nil
}

// Update applies update atomically. The schedule ID and creation timestamp are
// immutable; UpdatedAt is set after the callback succeeds.
func (s *Store) Update(ctx context.Context, id string, update func(*Schedule) error) (Schedule, error) {
	if err := validateID(id); err != nil {
		return Schedule{}, err
	}
	if update == nil {
		return Schedule{}, errors.New("schedulestore: update callback is required")
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: begin update: %w", err)
	}
	defer tx.Rollback()

	var data []byte
	if err := tx.QueryRowContext(ctx, `SELECT data FROM schedules WHERE id = ?`, id).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Schedule{}, ErrNotFound
		}
		return Schedule{}, fmt.Errorf("schedulestore: get schedule %q for update: %w", id, err)
	}
	schedule, err := decodeSchedule(id, data)
	if err != nil {
		return Schedule{}, err
	}
	createdAt := schedule.CreatedAt
	if err := update(&schedule); err != nil {
		return Schedule{}, err
	}
	if schedule.ID != id {
		return Schedule{}, errors.New("schedulestore: schedule ID cannot be changed")
	}
	schedule.CreatedAt = createdAt.UTC()
	schedule.UpdatedAt = time.Now().UTC()
	normalizeOptionalTimes(&schedule)
	encoded, err := json.Marshal(schedule)
	if err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: encode schedule: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE schedules
SET conversation_id = ?, created_at_ns = ?, updated_at_ns = ?, data = ?
WHERE id = ?`,
		schedule.ConversationID, schedule.CreatedAt.UnixNano(), schedule.UpdatedAt.UnixNano(), encoded, id)
	if err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: update schedule %q: %w", id, err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: inspect update %q: %w", id, err)
	} else if affected != 1 {
		return Schedule{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: commit update %q: %w", id, err)
	}
	return schedule, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	result, err := s.database.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("schedulestore: delete schedule %q: %w", id, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("schedulestore: inspect delete %q: %w", id, err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteForConversation removes every schedule attached to conversationID.
func (s *Store) DeleteForConversation(ctx context.Context, conversationID string) error {
	if strings.TrimSpace(conversationID) == "" {
		return errors.New("schedulestore: conversation ID is required")
	}
	if _, err := s.database.ExecContext(ctx, `DELETE FROM schedules WHERE conversation_id = ?`, conversationID); err != nil {
		return fmt.Errorf("schedulestore: delete schedules for conversation %q: %w", conversationID, err)
	}
	return nil
}

func validateID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("schedulestore: invalid schedule ID %q", id)
	}
	return nil
}

func decodeSchedule(id string, data []byte) (Schedule, error) {
	var schedule Schedule
	if err := json.Unmarshal(data, &schedule); err != nil {
		return Schedule{}, fmt.Errorf("schedulestore: decode schedule %q: %w", id, err)
	}
	if schedule.ID != id {
		return Schedule{}, fmt.Errorf("schedulestore: schedule %q contains mismatched ID %q", id, schedule.ID)
	}
	if err := validateID(schedule.ID); err != nil {
		return Schedule{}, err
	}
	schedule.CreatedAt = schedule.CreatedAt.UTC()
	schedule.UpdatedAt = schedule.UpdatedAt.UTC()
	normalizeOptionalTimes(&schedule)
	return schedule, nil
}

func normalizeOptionalTimes(schedule *Schedule) {
	schedule.RunAt = utcTime(schedule.RunAt)
	schedule.NextRunAt = utcTime(schedule.NextRunAt)
	schedule.LastRunAt = utcTime(schedule.LastRunAt)
}

func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}
