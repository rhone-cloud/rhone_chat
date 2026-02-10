package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

type Chat struct {
	ID        string
	Title     string
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Message struct {
	ID        string
	ChatID    string
	Role      string
	Content   string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Run struct {
	ID                 string
	ChatID             string
	UserMessageID      string
	AssistantMessageID string
	Model              string
	Status             string
	StopReason         string
	ErrorText          string
	ToolCallCount      int
	TurnCount          int
	UsageJSON          string
	StartedAt          time.Time
	FinishedAt         sql.NullTime
}

type ToolCall struct {
	ID         string
	RunID      string
	ToolCallID string
	Name       string
	Status     string
	InputJSON  string
	OutputJSON string
	ErrorText  string
	StartedAt  time.Time
	FinishedAt sql.NullTime
}

func OpenSQLite(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetConnMaxLifetime(0)

	store := &Store{db: database}
	if err := store.migrate(context.Background()); err != nil {
		database.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS chats (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  model TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  chat_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(chat_id) REFERENCES chats(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_messages_chat_created ON messages(chat_id, created_at, id);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  chat_id TEXT NOT NULL,
  user_message_id TEXT NOT NULL,
  assistant_message_id TEXT NOT NULL,
  model TEXT NOT NULL,
  status TEXT NOT NULL,
  stop_reason TEXT,
  error_text TEXT,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  turn_count INTEGER NOT NULL DEFAULT 0,
  usage_json TEXT,
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  FOREIGN KEY(chat_id) REFERENCES chats(id) ON DELETE CASCADE,
  FOREIGN KEY(user_message_id) REFERENCES messages(id) ON DELETE RESTRICT,
  FOREIGN KEY(assistant_message_id) REFERENCES messages(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_runs_chat_started ON runs(chat_id, started_at, id);

CREATE TABLE IF NOT EXISTS tool_calls (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  tool_call_id TEXT,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  input_json TEXT,
  output_json TEXT,
  error_text TEXT,
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_tool_calls_run_started ON tool_calls(run_id, started_at, id);
`
	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("migrate sqlite schema: %w", err)
	}
	return nil
}

func (s *Store) ListChats(ctx context.Context, limit int) ([]Chat, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, model, created_at, updated_at
FROM chats
ORDER BY updated_at DESC, id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	chats := make([]Chat, 0, limit)
	for rows.Next() {
		var chat Chat
		if err := rows.Scan(&chat.ID, &chat.Title, &chat.Model, &chat.CreatedAt, &chat.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		chats = append(chats, chat)
	}
	return chats, rows.Err()
}

func (s *Store) GetChat(ctx context.Context, chatID string) (Chat, error) {
	var chat Chat
	err := s.db.QueryRowContext(ctx, `
SELECT id, title, model, created_at, updated_at
FROM chats
WHERE id = ?`, chatID).Scan(&chat.ID, &chat.Title, &chat.Model, &chat.CreatedAt, &chat.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Chat{}, ErrNotFound
	}
	if err != nil {
		return Chat{}, fmt.Errorf("get chat: %w", err)
	}
	return chat, nil
}

func (s *Store) CreateChat(ctx context.Context, id, title, model string, now time.Time) (Chat, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chats (id, title, model, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, id, title, model, now, now)
	if err != nil {
		return Chat{}, fmt.Errorf("create chat: %w", err)
	}
	return Chat{ID: id, Title: title, Model: model, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) RenameChat(ctx context.Context, chatID, title string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE chats
SET title = ?, updated_at = ?
WHERE id = ?`, title, now, chatID)
	if err != nil {
		return fmt.Errorf("rename chat: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateChatModel(ctx context.Context, chatID, model string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE chats
SET model = ?, updated_at = ?
WHERE id = ?`, model, now, chatID)
	if err != nil {
		return fmt.Errorf("update chat model: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListMessages(ctx context.Context, chatID string, limit int) ([]Message, error) {
	if limit < 1 {
		limit = 300
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, chat_id, role, content, status, created_at, updated_at
FROM messages
WHERE chat_id = ?
ORDER BY created_at ASC, id ASC
LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	messages := make([]Message, 0, limit)
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.ChatID, &msg.Role, &msg.Content, &msg.Status, &msg.CreatedAt, &msg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *Store) InsertMessage(ctx context.Context, message Message) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO messages (id, chat_id, role, content, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, message.ID, message.ChatID, message.Role, message.Content, message.Status, message.CreatedAt, message.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (s *Store) UpdateMessageContent(ctx context.Context, messageID, content, status string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE messages
SET content = ?, status = ?, updated_at = ?
WHERE id = ?`, content, status, now, messageID)
	if err != nil {
		return fmt.Errorf("update message content: %w", err)
	}
	return nil
}

func (s *Store) UpsertRunStart(ctx context.Context, run Run) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runs (id, chat_id, user_message_id, assistant_message_id, model, status, started_at, tool_call_count, turn_count)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
status = excluded.status,
model = excluded.model,
chat_id = excluded.chat_id,
user_message_id = excluded.user_message_id,
assistant_message_id = excluded.assistant_message_id,
started_at = excluded.started_at`,
		run.ID, run.ChatID, run.UserMessageID, run.AssistantMessageID, run.Model, run.Status, run.StartedAt, run.ToolCallCount, run.TurnCount)
	if err != nil {
		return fmt.Errorf("upsert run start: %w", err)
	}
	return nil
}

func (s *Store) CompleteRun(ctx context.Context, runID, status, stopReason, errorText string, toolCallCount, turnCount int, usage any, finishedAt time.Time) error {
	usageBytes, err := json.Marshal(usage)
	if err != nil {
		usageBytes = []byte("{}")
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE runs
SET status = ?, stop_reason = ?, error_text = ?, tool_call_count = ?, turn_count = ?, usage_json = ?, finished_at = ?
WHERE id = ?`, status, stopReason, errorText, toolCallCount, turnCount, string(usageBytes), finishedAt, runID)
	if err != nil {
		return fmt.Errorf("complete run: %w", err)
	}
	return nil
}

func (s *Store) UpsertToolCallStart(ctx context.Context, call ToolCall) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tool_calls (id, run_id, tool_call_id, name, status, input_json, started_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
status = excluded.status,
input_json = excluded.input_json,
name = excluded.name,
tool_call_id = excluded.tool_call_id`,
		call.ID, call.RunID, call.ToolCallID, call.Name, call.Status, call.InputJSON, call.StartedAt)
	if err != nil {
		return fmt.Errorf("upsert tool call start: %w", err)
	}
	return nil
}

func (s *Store) CompleteToolCall(ctx context.Context, callID, status, outputJSON, errorText string, finishedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE tool_calls
SET status = ?, output_json = ?, error_text = ?, finished_at = ?
WHERE id = ?`, status, outputJSON, errorText, finishedAt, callID)
	if err != nil {
		return fmt.Errorf("complete tool call: %w", err)
	}
	return nil
}

func (s *Store) TouchChat(ctx context.Context, chatID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE chats
SET updated_at = ?
WHERE id = ?`, at, chatID)
	if err != nil {
		return fmt.Errorf("touch chat: %w", err)
	}
	return nil
}

func (s *Store) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func InsertMessageTx(ctx context.Context, tx *sql.Tx, message Message) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO messages (id, chat_id, role, content, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, message.ID, message.ChatID, message.Role, message.Content, message.Status, message.CreatedAt, message.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert message tx: %w", err)
	}
	return nil
}

func UpsertRunStartTx(ctx context.Context, tx *sql.Tx, run Run) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO runs (id, chat_id, user_message_id, assistant_message_id, model, status, started_at, tool_call_count, turn_count)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
status = excluded.status,
model = excluded.model,
chat_id = excluded.chat_id,
user_message_id = excluded.user_message_id,
assistant_message_id = excluded.assistant_message_id,
started_at = excluded.started_at`,
		run.ID, run.ChatID, run.UserMessageID, run.AssistantMessageID, run.Model, run.Status, run.StartedAt, run.ToolCallCount, run.TurnCount)
	if err != nil {
		return fmt.Errorf("upsert run start tx: %w", err)
	}
	return nil
}

func TouchChatTx(ctx context.Context, tx *sql.Tx, chatID string, at time.Time) error {
	_, err := tx.ExecContext(ctx, `
UPDATE chats SET updated_at = ? WHERE id = ?`, at, chatID)
	if err != nil {
		return fmt.Errorf("touch chat tx: %w", err)
	}
	return nil
}

func CreateChatTx(ctx context.Context, tx *sql.Tx, id, title, model string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO chats (id, title, model, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, id, title, model, now, now)
	if err != nil {
		return fmt.Errorf("create chat tx: %w", err)
	}
	return nil
}
