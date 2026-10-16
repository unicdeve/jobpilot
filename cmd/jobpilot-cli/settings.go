package main

import (
	"context"
	"fmt"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/llm"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

func newSettingsCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "settings", Short: "Manage JobPilot settings"}
	cmd.AddCommand(newNotificationSettingsCmd(client))
	cmd.AddCommand(newPromptSettingsCmd(client))

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show the configured LLM provider and model",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			settings, err := client.GetLLMSettings(ctx)
			if err != nil {
				return fmt.Errorf("loading LLM settings: %w", err)
			}

			resolved := llm.ResolveSettings(*settings)
			note := ""
			if settings.Provider == "" {
				note = " (default, not yet configured)"
			}
			fmt.Printf("Provider: %s%s\nBase URL: %s\nModel: %s\n", resolved.Provider, note, resolved.BaseURL, resolved.Model)
			if ref, ok := resolved.APIKeyRef(); ok {
				where, err := keyStatus(ctx, client, ref)
				if err != nil {
					return err
				}
				fmt.Printf("API key: %s (%s)\n", ref.Label, where)
			}
			if host := resolved.HostedAt(); host != "" {
				fmt.Printf("\n%s\n", domain.HostedModelNotice(host))
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set-llm",
		Short: "Interactively configure the LLM provider and model",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			labels := make([]string, len(llm.Presets))
			for i, p := range llm.Presets {
				labels[i] = p.Label
			}
			providerSelect := promptui.Select{Label: "LLM Provider", Items: labels}
			idx, _, err := providerSelect.Run()
			if err != nil {
				return fmt.Errorf("selecting provider: %w", err)
			}
			preset := llm.Presets[idx]

			urlPrompt := promptui.Prompt{Label: "Base URL", Default: preset.DefaultBaseURL}
			baseURL, err := urlPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for base URL: %w", err)
			}

			modelPrompt := promptui.Prompt{
				Label: "Model name",
				Validate: func(v string) error {
					if v == "" {
						return fmt.Errorf("model name is required")
					}
					return nil
				},
			}
			model, err := modelPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for model: %w", err)
			}

			settings := domain.LLMSettings{Provider: preset.Key, BaseURL: baseURL, Model: model}
			if host := settings.HostedAt(); host != "" {
				fmt.Printf("ℹ️  %s\n", domain.HostedModelNotice(host))
			}
			if preset.Key == llm.ProviderCustom {
				label, err := chooseLLMKey(ctx, client)
				if err != nil {
					return err
				}
				settings.APIKeyLabel = label
			}

			result, err := client.SaveLLMSettings(ctx, settings)
			if err != nil {
				return fmt.Errorf("saving LLM settings: %w", err)
			}
			if result.ConnectivityError != "" {
				fmt.Printf("⚠️  Settings saved, but could not connect: %s\n", result.ConnectivityError)
				return nil
			}
			fmt.Println("✅ Settings saved and verified.")
			return nil
		},
	})

	return cmd
}

// keyStatus says whether the credential ref names is stored, as the list
// of stored credentials records it; the secret itself is never read.
func keyStatus(ctx context.Context, client *httpapi.Client, ref domain.CredentialRef) (string, error) {
	refs, err := client.ListCredentials(ctx)
	if err != nil {
		return "", fmt.Errorf("listing API keys: %w", err)
	}
	for _, stored := range refs {
		if stored == ref {
			return "in your keychain", nil
		}
	}
	return "not stored — add it with 'settings set-llm'", nil
}

// chooseLLMKey asks which API key a custom endpoint is sent: none, one
// already stored for the model, or a new one, which it stores (ADR-0030
// §7). It returns the key's label, or "" for none.
func chooseLLMKey(ctx context.Context, client *httpapi.Client) (string, error) {
	refs, err := client.ListCredentials(ctx)
	if err != nil {
		return "", fmt.Errorf("listing API keys: %w", err)
	}
	const none, add = "No key", "Add a new key…"
	items := []string{none}
	for _, ref := range refs {
		// Only the model's own keys: a job source's is not offered (§1).
		if ref.Source == domain.LLMCredentialSource {
			items = append(items, ref.Label)
		}
	}
	items = append(items, add)

	keySelect := promptui.Select{Label: "API key", Items: items}
	_, choice, err := keySelect.Run()
	if err != nil {
		return "", fmt.Errorf("choosing an API key: %w", err)
	}
	if choice == none {
		return "", nil
	}
	if choice != add {
		return choice, nil
	}

	labelPrompt := promptui.Prompt{
		Label: "Name this key (e.g. openrouter)",
		Validate: func(v string) error {
			_, err := domain.NewCredentialRef(domain.LLMCredentialSource, v)
			return err
		},
	}
	label, err := labelPrompt.Run()
	if err != nil {
		return "", fmt.Errorf("naming the key: %w", err)
	}
	secret, err := readSecret("API key")
	if err != nil {
		return "", err
	}
	ref, err := client.SetCredential(ctx, domain.LLMCredentialSource, label, secret)
	if err != nil {
		return "", fmt.Errorf("storing the key: %w", err)
	}
	fmt.Printf("✅ Stored the key %q in your keychain. It cannot be shown again.\n", ref.Label)
	return ref.Label, nil
}
