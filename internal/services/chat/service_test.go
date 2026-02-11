package chat

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"rhone_chat/internal/config"
	"rhone_chat/internal/db"
)

func TestRenameChatTrimsAndPersists(t *testing.T) {
	store := newTestStore(t)
	service := newTestService(store)
	now := time.Now().UTC()

	created, err := store.CreateChat(context.Background(), "chat-1", "Original title", config.DefaultModel, now)
	if err != nil {
		t.Fatalf("CreateChat() error = %v", err)
	}

	err = service.RenameChat(context.Background(), created.ID, "   Renamed title   ")
	if err != nil {
		t.Fatalf("RenameChat() error = %v", err)
	}

	updated, err := store.GetChat(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetChat() error = %v", err)
	}
	if updated.Title != "Renamed title" {
		t.Fatalf("updated.Title = %q, want %q", updated.Title, "Renamed title")
	}
}

func TestRenameChatRejectsEmptyTitle(t *testing.T) {
	store := newTestStore(t)
	service := newTestService(store)

	err := service.RenameChat(context.Background(), "chat-1", "   ")
	if err == nil {
		t.Fatalf("RenameChat() expected error for empty title")
	}
}

func TestDeleteChatRemovesChat(t *testing.T) {
	store := newTestStore(t)
	service := newTestService(store)
	now := time.Now().UTC()

	created, err := store.CreateChat(context.Background(), "chat-1", "A chat", config.DefaultModel, now)
	if err != nil {
		t.Fatalf("CreateChat() error = %v", err)
	}

	err = service.DeleteChat(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("DeleteChat() error = %v", err)
	}

	_, err = store.GetChat(context.Background(), created.ID)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetChat() error = %v, want ErrNotFound", err)
	}
}

func TestDeleteChatMissingReturnsNotFound(t *testing.T) {
	store := newTestStore(t)
	service := newTestService(store)

	err := service.DeleteChat(context.Background(), "missing-chat")
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("DeleteChat() error = %v, want ErrNotFound", err)
	}
}

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.OpenSQLite(filepath.Join(t.TempDir(), "chat.sqlite"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func newTestService(store *db.Store) *Service {
	return NewService(store, nil, config.Config{
		DefaultModel: config.DefaultModel,
		MaxHistory:   30,
		SystemPrompt: "You are helpful.",
	})
}
