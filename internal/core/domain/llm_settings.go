package domain

// LLMSettings is the user's configured LLM backend: which provider, where
// to reach it, and which model to use.
type LLMSettings struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}

// DiscoverySettings is how discovery behaves without being asked.
type DiscoverySettings struct {
	// RefreshOnLaunch is whether the desktop runs the saved searches gone
	// stale (SavedSearch.Stale) when it opens. On unless turned off.
	RefreshOnLaunch bool `json:"refresh_on_launch"`
}
