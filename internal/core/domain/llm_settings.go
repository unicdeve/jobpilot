package domain

// LLMSettings is the user's configured LLM backend: which provider, where
// to reach it, and which model to use.
type LLMSettings struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}
