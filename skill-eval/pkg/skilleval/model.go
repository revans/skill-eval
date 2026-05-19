package skilleval

import (
	"fmt"
	"strings"
)

// ParseModelList splits a comma-separated model string into individual model names.
// Whitespace around entries is trimmed; empty entries after trimming are dropped.
func ParseModelList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ResolveModels returns the model list for a targeted run using the four-tier fallback:
//  1. modelFlag (non-empty) — explicit --model list overrides everything
//  2. allModels flag — firstEval's primary + all secondaries
//  3. firstEval.Models.Primary declared — the eval's preferred primary model
//  4. defaultModel — the config default
func ResolveModels(firstEval Eval, modelFlag []string, allModels bool, defaultModel string) ([]string, error) {
	if len(modelFlag) > 0 {
		return modelFlag, nil
	}
	if allModels {
		if firstEval.Models == nil {
			return nil, fmt.Errorf(
				"--all-models requires the eval to declare a models: block.\n%s has no models declared. Use --model to specify explicitly,\nor omit --all-models to use the default model.",
				firstEval.ID,
			)
		}
		models := []string{firstEval.Models.Primary}
		models = append(models, firstEval.Models.Secondaries...)
		return models, nil
	}
	if firstEval.Models != nil && firstEval.Models.Primary != "" {
		return []string{firstEval.Models.Primary}, nil
	}
	return []string{defaultModel}, nil
}
