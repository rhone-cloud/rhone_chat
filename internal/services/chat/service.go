package chat

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"rhone_chat/internal/ai"
	"rhone_chat/internal/config"
	"rhone_chat/internal/db"
)

type Service struct {
	store  *db.Store
	runner *ai.Runner
	cfg    config.Config
}

type Chat = db.Chat
type Message = db.Message
type ToolCall = db.ToolCall

type AIMessage = ai.Message
type StreamCallbacks = ai.StreamCallbacks
type StreamResult = ai.StreamResult
type ToolCallUpdate = ai.ToolCallUpdate

type PendingRun struct {
	RunID              string
	ChatID             string
	UserMessageID      string
	AssistantMessageID string
	Model              string
}

func NewService(store *db.Store, runner *ai.Runner, cfg config.Config) *Service {
	return &Service{store: store, runner: runner, cfg: cfg}
}

func (s *Service) DefaultModel() string {
	return s.cfg.DefaultModel
}

func (s *Service) AllowedModels() []string {
	return ai.AllowedModels
}

func (s *Service) IsAllowedModel(model string) bool {
	return ai.IsAllowedModel(model)
}

func (s *Service) ListOrCreateChats(ctx context.Context, limit int) ([]Chat, error) {
	chatList, err := s.store.ListChats(ctx, limit)
	if err != nil {
		return nil, err
	}
	if len(chatList) > 0 {
		return chatList, nil
	}
	newChatID := uuid.NewString()
	now := time.Now().UTC()
	created, err := s.store.CreateChat(ctx, newChatID, "New chat", s.cfg.DefaultModel, now)
	if err != nil {
		return nil, err
	}
	return []Chat{created}, nil
}

func (s *Service) ListMessages(ctx context.Context, chatID string, limit int) ([]Message, error) {
	if chatID == "" {
		return nil, nil
	}
	return s.store.ListMessages(ctx, chatID, limit)
}

func (s *Service) CreateChat(ctx context.Context, model string) (Chat, error) {
	if !ai.IsAllowedModel(model) {
		model = s.cfg.DefaultModel
	}
	now := time.Now().UTC()
	return s.store.CreateChat(ctx, uuid.NewString(), "New chat", model, now)
}

func (s *Service) RenameChat(ctx context.Context, chatID, title string) error {
	trimmedChatID := strings.TrimSpace(chatID)
	if trimmedChatID == "" {
		return errors.New("chat id is required")
	}
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return errors.New("chat title cannot be empty")
	}
	if len(trimmedTitle) > 200 {
		return errors.New("chat title is too long")
	}
	return s.store.RenameChat(ctx, trimmedChatID, trimmedTitle, time.Now().UTC())
}

func (s *Service) DeleteChat(ctx context.Context, chatID string) error {
	trimmedChatID := strings.TrimSpace(chatID)
	if trimmedChatID == "" {
		return errors.New("chat id is required")
	}
	return s.store.DeleteChat(ctx, trimmedChatID)
}

func (s *Service) PersistRunStart(ctx context.Context, run PendingRun, userMessageContent string) error {
	now := time.Now().UTC()
	err := s.store.Transaction(ctx, func(tx *sql.Tx) error {
		if txErr := db.InsertMessageTx(ctx, tx, db.Message{
			ID:        run.UserMessageID,
			ChatID:    run.ChatID,
			Role:      "user",
			Content:   userMessageContent,
			Status:    "complete",
			CreatedAt: now,
			UpdatedAt: now,
		}); txErr != nil {
			return txErr
		}
		if txErr := db.InsertMessageTx(ctx, tx, db.Message{
			ID:        run.AssistantMessageID,
			ChatID:    run.ChatID,
			Role:      "assistant",
			Content:   "",
			Status:    "streaming",
			CreatedAt: now,
			UpdatedAt: now,
		}); txErr != nil {
			return txErr
		}
		if txErr := db.UpsertRunStartTx(ctx, tx, db.Run{
			ID:                 run.RunID,
			ChatID:             run.ChatID,
			UserMessageID:      run.UserMessageID,
			AssistantMessageID: run.AssistantMessageID,
			Model:              run.Model,
			Status:             "running",
			StartedAt:          now,
		}); txErr != nil {
			return txErr
		}
		if txErr := db.TouchChatTx(ctx, tx, run.ChatID, now); txErr != nil {
			return txErr
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.store.UpdateChatModel(ctx, run.ChatID, run.Model, now)
}

func (s *Service) BuildHistory(ctx context.Context, chatID string) ([]AIMessage, error) {
	rows, err := s.store.ListMessages(ctx, chatID, 800)
	if err != nil {
		return nil, err
	}
	history := make([]AIMessage, 0, s.cfg.MaxHistory+1)
	history = append(history, AIMessage{Role: "system", Content: s.cfg.SystemPrompt})
	for _, row := range rows {
		if row.Role != "user" && row.Role != "assistant" {
			continue
		}
		if row.Role == "assistant" && strings.TrimSpace(row.Content) == "" {
			continue
		}
		history = append(history, AIMessage{Role: row.Role, Content: row.Content})
	}
	if len(history) <= s.cfg.MaxHistory+1 {
		return history, nil
	}
	trimmed := make([]AIMessage, 0, s.cfg.MaxHistory+1)
	trimmed = append(trimmed, history[0])
	trimmed = append(trimmed, history[len(history)-s.cfg.MaxHistory:]...)
	return trimmed, nil
}

func (s *Service) Stream(ctx context.Context, model string, history []AIMessage, callbacks StreamCallbacks) (StreamResult, error) {
	return s.runner.Stream(ctx, model, history, callbacks)
}

func (s *Service) UpdateAssistantPartial(ctx context.Context, assistantMessageID, content string) error {
	return s.store.UpdateMessageContent(ctx, assistantMessageID, content, "streaming", time.Now().UTC())
}

func (s *Service) CompleteAssistant(ctx context.Context, assistantMessageID, content, status string) error {
	return s.store.UpdateMessageContent(ctx, assistantMessageID, content, status, time.Now().UTC())
}

func (s *Service) UpsertToolStart(ctx context.Context, runID string, update ToolCallUpdate) (string, error) {
	callID := uuid.NewString()
	err := s.store.UpsertToolCallStart(ctx, db.ToolCall{
		ID:         callID,
		RunID:      runID,
		ToolCallID: update.ID,
		Name:       update.Name,
		Status:     "running",
		InputJSON:  truncateText(update.Input, 4000),
		StartedAt:  time.Now().UTC(),
	})
	return callID, err
}

func (s *Service) CompleteTool(ctx context.Context, callID string, update ToolCallUpdate) error {
	status := update.Status
	if status == "" {
		status = "completed"
	}
	return s.store.CompleteToolCall(ctx, callID, status, truncateText(update.Output, 4000), truncateText(update.ErrText, 2000), time.Now().UTC())
}

func (s *Service) CompleteRun(ctx context.Context, run PendingRun, status string, result StreamResult, errText string) error {
	if err := s.store.CompleteRun(ctx, run.RunID, status, result.StopReason, errText, result.ToolCallCount, result.TurnCount, result.Usage, time.Now().UTC()); err != nil {
		return err
	}
	return s.store.TouchChat(ctx, run.ChatID, time.Now().UTC())
}

func (s *Service) IsCancellation(err error, ctx context.Context) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return true
	}
	return false
}

func (s *Service) FlushConfig() (time.Duration, int, time.Duration) {
	return s.cfg.UIFlushInterval, s.cfg.UIFlushBytes, s.cfg.DBFlushInterval
}

func truncateText(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 3 {
		return value[:maxBytes]
	}
	return value[:maxBytes-3] + "..."
}
