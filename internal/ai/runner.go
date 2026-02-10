package ai

import (
	"context"
	"encoding/json"
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

	requestMessages := make([]vai.Message, 0, len(messages))
	for _, message := range messages {
		requestMessages = append(requestMessages, vai.Message{
			Role:    message.Role,
			Content: []vai.ContentBlock{vai.Text(message.Content)},
		})
	}

	req := &vai.MessageRequest{
		Model:    model,
		Messages: requestMessages,
		Tools: []vai.Tool{
			vai.WebSearch(),
		},
		ToolChoice: vai.ToolChoiceAuto(),
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
		return StreamResult{}, err
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
		return StreamResult{}, processErr
	}
	if err := stream.Err(); err != nil {
		return StreamResult{}, err
	}

	final := stream.Result()
	return StreamResult{
		StopReason:    string(final.StopReason),
		ToolCallCount: final.ToolCallCount,
		TurnCount:     final.TurnCount,
		Usage:         final.Usage,
	}, nil
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
