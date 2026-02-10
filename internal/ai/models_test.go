package ai

import "testing"

func TestResolveModelAnthropicAlias(t *testing.T) {
	got := ResolveModel("anthropic/claude-haiku-4-5")
	want := "anthropic/claude-haiku-4-5-20251001"
	if got != want {
		t.Fatalf("ResolveModel() = %q, want %q", got, want)
	}
}

func TestResolveModelNoAlias(t *testing.T) {
	got := ResolveModel("oai-resp/gpt-5-mini")
	want := "oai-resp/gpt-5-mini"
	if got != want {
		t.Fatalf("ResolveModel() = %q, want %q", got, want)
	}
}
