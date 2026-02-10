package ai

var AllowedModels = []string{
	"oai-resp/gpt-5-mini",
	"gemini/gemini-3-flash-preview",
	"anthropic/claude-haiku-4-5",
}

var canonicalModelMap = map[string]string{
	"anthropic/claude-haiku-4-5": "anthropic/claude-haiku-4-5-20251001",
}

func IsAllowedModel(model string) bool {
	for _, candidate := range AllowedModels {
		if model == candidate {
			return true
		}
	}
	return false
}

func ResolveModel(model string) string {
	if resolved, ok := canonicalModelMap[model]; ok && resolved != "" {
		return resolved
	}
	return model
}
