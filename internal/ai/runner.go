package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	vai "github.com/vango-go/vai-lite/sdk"
)

type Message struct {
	Role    string
	Content string
}

type RunnerConfig struct {
	MaxTurns     int
	MaxToolCalls int
	RunTimeout   time.Duration
	ToolTimeout  time.Duration
}

type Runner struct {
	client *vai.Client
	cfg    RunnerConfig
}

type ToolCallUpdate struct {
	ID      string
	Name    string
	Status  string
	Input   string
	Output  string
	ErrText string
}

type StreamCallbacks struct {
	OnTextDelta  func(string)
	OnThinking   func()
	OnToolStart  func(ToolCallUpdate)
	OnToolResult func(ToolCallUpdate)
}

type StreamResult struct {
	StopReason    string
	ToolCallCount int
	TurnCount     int
	Usage         any
}

func NewRunner(cfg RunnerConfig) *Runner {
	client := vai.NewClient()
	return &Runner{client: client, cfg: cfg}
}

func (r *Runner) Stream(ctx context.Context, model string, messages []Message, callbacks StreamCallbacks) (StreamResult, error) {
	if !IsAllowedModel(model) {
		return StreamResult{}, fmt.Errorf("unsupported model %q", model)
	}
	resolvedModel := ResolveModel(model)

	requestMessages, systemPrompt := normalizeMessagesForRequest(messages)

	req := &vai.MessageRequest{
		Model:    resolvedModel,
		Messages: requestMessages,
		Tools: []vai.Tool{
			vai.WebSearch(),
		},
		ToolChoice: vai.ToolChoiceAuto(),
	}
	if systemPrompt != "" {
		req.System = systemPrompt
	}

	runCtx := ctx
	cancel := func() {}
	if r.cfg.RunTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.cfg.RunTimeout)
	}
	defer cancel()

	opts := []vai.RunOption{}
	if r.cfg.MaxTurns > 0 {
		opts = append(opts, vai.WithMaxTurns(r.cfg.MaxTurns))
	}
	if r.cfg.MaxToolCalls > 0 {
		opts = append(opts, vai.WithMaxToolCalls(r.cfg.MaxToolCalls))
	}
	if r.cfg.ToolTimeout > 0 {
		opts = append(opts, vai.WithToolTimeout(r.cfg.ToolTimeout))
	}

	stream, err := r.client.Messages.RunStream(runCtx, req, opts...)
	if err != nil {
		return StreamResult{}, wrapStreamError(model, resolvedModel, "start", err)
	}
	defer stream.Close()

	_, processErr := stream.Process(vai.StreamCallbacks{
		OnTextDelta: func(delta string) {
			if callbacks.OnTextDelta != nil {
				callbacks.OnTextDelta(delta)
			}
		},
		OnThinkingDelta: func(delta string) {
			if callbacks.OnThinking != nil && strings.TrimSpace(delta) != "" {
				callbacks.OnThinking()
			}
		},
		OnToolCallStart: func(id, name string, input map[string]any) {
			if callbacks.OnToolStart == nil {
				return
			}
			encoded, _ := json.Marshal(input)
			callbacks.OnToolStart(ToolCallUpdate{
				ID:     id,
				Name:   name,
				Status: "running",
				Input:  string(encoded),
			})
		},
		OnToolResult: func(id, name string, content []vai.ContentBlock, toolErr error) {
			if callbacks.OnToolResult == nil {
				return
			}
			update := ToolCallUpdate{
				ID:     id,
				Name:   name,
				Status: "completed",
				Output: contentBlocksToText(content),
			}
			if toolErr != nil {
				update.Status = "error"
				update.ErrText = toolErr.Error()
			}
			callbacks.OnToolResult(update)
		},
	})
	if processErr != nil {
		return StreamResult{}, wrapStreamError(model, resolvedModel, "process", processErr)
	}
	if err := stream.Err(); err != nil {
		return StreamResult{}, wrapStreamError(model, resolvedModel, "stream", err)
	}

	final := stream.Result()
	stopReason := string(final.StopReason)
	if stopReason == "error" {
		return StreamResult{}, fmt.Errorf("ai stream failed for model %q (provider model %q): stop_reason=error", model, resolvedModel)
	}

	return StreamResult{
		StopReason:    stopReason,
		ToolCallCount: final.ToolCallCount,
		TurnCount:     final.TurnCount,
		Usage:         final.Usage,
	}, nil
}

func wrapStreamError(selectedModel, providerModel, stage string, err error) error {
	if err == nil {
		return fmt.Errorf("ai stream failed for model %q at %s", selectedModel, stage)
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "provider returned an empty error"
	}
	return fmt.Errorf("ai stream failed for model %q (provider model %q) at %s: %s", selectedModel, providerModel, stage, message)
}

func contentBlocksToText(blocks []vai.ContentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		raw, err := json.Marshal(block)
		if err != nil {
			continue
		}
		parts = append(parts, string(raw))
	}
	return strings.Join(parts, "\n")
}

func normalizeMessagesForRequest(messages []Message) ([]vai.Message, string) {
	requestMessages := make([]vai.Message, 0, len(messages))
	systemParts := make([]string, 0, 1)
	for _, message := range messages {
		if message.Role == "system" {
			systemText := strings.TrimSpace(message.Content)
			if systemText != "" {
				systemParts = append(systemParts, systemText)
			}
			continue
		}
		requestMessages = append(requestMessages, vai.Message{
			Role:    message.Role,
			Content: []vai.ContentBlock{vai.Text(message.Content)},
		})
	}
	return requestMessages, strings.Join(systemParts, "\n\n")
}
