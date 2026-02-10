package ai

import "testing"

func TestNormalizeMessagesForRequest_ExtractsSystemPrompt(t *testing.T) {
	input := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "system", Content: "Use web search if needed."},
	}

	requestMessages, systemPrompt := normalizeMessagesForRequest(input)

	if systemPrompt != "You are helpful.\n\nUse web search if needed." {
		t.Fatalf("systemPrompt = %q", systemPrompt)
	}
	if len(requestMessages) != 2 {
		t.Fatalf("len(requestMessages) = %d, want 2", len(requestMessages))
	}
	if requestMessages[0].Role != "user" {
		t.Fatalf("requestMessages[0].Role = %q, want user", requestMessages[0].Role)
	}
	if requestMessages[1].Role != "assistant" {
		t.Fatalf("requestMessages[1].Role = %q, want assistant", requestMessages[1].Role)
	}
}
