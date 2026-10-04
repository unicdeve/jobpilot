package domain

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// LLMCredentialSource is the credential source the model's API keys are
// stored under (ADR-0030 §1). The label is the user's.
const LLMCredentialSource = "llm"

// LLMProviderOllama is Ollama's provider key. Ollama holds its own sign-in
// for its cloud models, so it is never given an API key (ADR-0019).
const LLMProviderOllama = "ollama"

// LLMSettings is the user's configured LLM backend: which provider, where
// to reach it, and which model to use.
type LLMSettings struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	// APIKeyLabel names the credential, under LLMCredentialSource, sent
	// with every request; empty sends none. It is the label and never the
	// key, which stays in the keychain (ADR-0030 §1).
	APIKeyLabel string `json:"api_key_label,omitempty"`
}

// APIKeyRef is the credential the settings name, and whether they name one.
func (s LLMSettings) APIKeyRef() (CredentialRef, bool) {
	if s.APIKeyLabel == "" {
		return CredentialRef{}, false
	}
	return CredentialRef{Source: LLMCredentialSource, Label: s.APIKeyLabel}, true
}

// Validate reports whether the settings can be saved.
func (s LLMSettings) Validate() error {
	if s.Provider == "" {
		return errors.New("provider is required")
	}
	if s.Model == "" {
		return errors.New("model is required")
	}

	parsed, err := url.ParseRequestURI(s.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("base URL must be a valid absolute http(s) URL, got %q", s.BaseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("base URL must use http or https, got %q", s.BaseURL)
	}

	ref, ok := s.APIKeyRef()
	if !ok {
		return nil
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("API key: %w", err)
	}
	if s.Provider == LLMProviderOllama {
		return errors.New("an API key is for an OpenAI-compatible endpoint; Ollama keeps its own sign-in and takes none")
	}
	// A key crosses the network only encrypted (ADR-0030 §3). A local
	// server started with an API key still works over plain http.
	if parsed.Scheme == "http" && !IsLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("an API key is sent only over https, or to this computer; %s would send it unencrypted", parsed.Host)
	}
	return nil
}

// HostedAt is the host the model is reached at, when that is not this
// computer: where the profile, resume and job postings are sent whenever
// JobPilot reads, scores or writes (ADR-0030 §6). It is empty for a model
// on this computer, and for a base URL that does not parse.
func (s LLMSettings) HostedAt() string {
	parsed, err := url.Parse(s.BaseURL)
	if err != nil || parsed.Hostname() == "" || IsLoopbackHost(parsed.Hostname()) {
		return ""
	}
	return parsed.Hostname()
}

// HostedModelNotice is what the user is told about a model reached at
// host, wherever they choose it.
func HostedModelNotice(host string) string {
	return fmt.Sprintf(
		"Your profile, resume and the job postings JobPilot reads are sent to %s whenever it reads, scores or writes, including when a scout runs on its own.",
		host)
}

// IsLoopbackHost reports whether host names this computer: "localhost", or
// an address in 127.0.0.0/8 or ::1. A name that only resolves to one is
// not counted, since what it resolves to can change.
func IsLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// DiscoverySettings is how discovery behaves without being asked.
type DiscoverySettings struct {
	// RefreshOnLaunch is whether the desktop runs the saved searches gone
	// stale (SavedSearch.Stale) when it opens. On unless turned off.
	RefreshOnLaunch bool `json:"refresh_on_launch"`
}
