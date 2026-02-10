package ai

var AllowedModels = []string{
	"oai-resp/gpt-5-mini",
	"gemini/gemini-3-flash-preview",
	"anthropic/claude-haiku-4-5",
}

func IsAllowedModel(model string) bool {
	for _, candidate := range AllowedModels {
		if model == candidate {
			return true
		}
	}
	return false
}
