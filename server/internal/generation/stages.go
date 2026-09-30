package generation

import (
	"context"
	"fmt"

	"github.com/ferousco-dev/layr/server/internal/account"
)

// ProviderLabel is how a provider's models are named to people.
func ProviderLabel(provider string) string {
	switch provider {
	case account.Anthropic:
		return "Claude (Anthropic)"
	case account.OpenAI:
		return "GPT (OpenAI)"
	case account.XAI:
		return "Grok (xAI)"
	}
	return provider
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func fixed(text string) func(string) string { return func(string) string { return text } }

// DefaultStages are the steps that do real work today. Writing the code itself arrives with the AI milestone.
func DefaultStages(designs Designs, keys Keys) []Stage {
	return []Stage{
		{
			ID: "read_design", Label: fixed("Reading your design"),
			Run: func(ctx context.Context, job *Job) (string, error) {
				d, err := designs.Design(ctx, job.UserID, job.ProjectID)
				if err != nil {
					return "", err
				}
				if d.Counts == nil {
					return "", ErrDesignNotReady
				}
				job.Design = d
				screens := plural(d.Counts.Screens, "screen", "screens")
				if job.ScreenIDs != nil {
					screens = fmt.Sprintf("%d of %s selected", len(job.ScreenIDs), plural(d.Counts.Screens, "screen", "screens"))
				}
				return fmt.Sprintf("%s, %s", screens, plural(d.Counts.SharedComponents, "shared component", "shared components")), nil
			},
		},
		{
			ID: "extract_tokens", Label: fixed("Extracting colors and fonts"),
			Run: func(ctx context.Context, job *Job) (string, error) {
				t, err := designs.Tokens(ctx, job.UserID, job.ProjectID)
				if err != nil {
					return "", err
				}
				job.Tokens = t
				return fmt.Sprintf("%s, %s", plural(len(t.Colors), "color", "colors"), plural(len(t.Fonts), "font", "fonts")), nil
			},
		},
		{
			ID: "prepare_model", Label: func(p string) string { return "Unlocking your " + ProviderLabel(p) + " key" },
			Run: func(ctx context.Context, job *Job) (string, error) {
				key, err := keys.OpenKey(ctx, job.UserID, job.Provider)
				if err != nil {
					return "", err
				}
				return "Key ending in " + account.Hint(key) + " is ready", nil
			},
		},
		{
			ID: "write_code", Label: func(p string) string { return "Writing code with " + ProviderLabel(p) },
			Run: func(context.Context, *Job) (string, error) {
				return "Code generation is not available yet", ErrStageUnavailable
			},
		},
	}
}
