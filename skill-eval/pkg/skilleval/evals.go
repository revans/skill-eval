package skilleval

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Eval is a fully validated eval definition ready for execution.
type Eval struct {
	ID         string
	Tests      string
	Prompt     string
	PromptFile string
	Input      string
	Assert     []Assertion
	Models     *ModelsBlock
}

// ModelsBlock holds the optional model declarations from an eval definition.
type ModelsBlock struct {
	Primary     string
	Secondaries []string
}

// Assertion is a parsed and validated assertion from an eval's assert list.
type Assertion struct {
	Type  string
	Value string
}

// rawEval is the YAML-deserialization form of an eval entry.
type rawEval struct {
	ID         string              `yaml:"id"`
	Tests      string              `yaml:"tests"`
	Prompt     string              `yaml:"prompt"`
	PromptFile string              `yaml:"prompt_file"`
	Input      string              `yaml:"input"`
	Assert     []map[string]string `yaml:"assert"`
	Models     *rawModels          `yaml:"models"`
}

type rawModels struct {
	Primary     string   `yaml:"primary"`
	Secondaries []string `yaml:"secondaries"`
}

var knownAssertionTypes = map[string]bool{
	"contains":     true,
	"not_contains": true,
	"matches":      true,
	"not_matches":  true,
}

// LoadEvals reads and validates the evals file at path.
// All validation errors reference the offending eval ID.
func LoadEvals(path string) ([]Eval, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading evals file %q: %w", path, err)
	}

	var raws []rawEval
	if err := yaml.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("parsing evals file %q: %w", path, err)
	}

	seenIDs := make(map[string]int) // id → position
	evals := make([]Eval, 0, len(raws))

	for i, r := range raws {
		pos := i + 1

		evalID := r.ID
		if evalID == "" {
			return nil, fmt.Errorf("eval at position %d is missing required field: id", pos)
		}
		if prev, seen := seenIDs[evalID]; seen {
			return nil, fmt.Errorf("duplicate eval ID %s (first at position %d, current at position %d)", evalID, prev, pos)
		}
		seenIDs[evalID] = pos

		if r.Tests == "" {
			return nil, fmt.Errorf("%s is missing required field: tests", evalID)
		}
		if r.Input == "" {
			return nil, fmt.Errorf("%s is missing required field: input", evalID)
		}

		if r.Prompt != "" && r.PromptFile != "" {
			return nil, fmt.Errorf("%s has both `prompt:` and `prompt_file:` set. Specify exactly one.", evalID)
		}
		if r.Prompt == "" && r.PromptFile == "" {
			return nil, fmt.Errorf("%s has neither `prompt:` nor `prompt_file:` set. Specify one.", evalID)
		}
		if r.PromptFile != "" {
			if _, err := os.Stat(r.PromptFile); os.IsNotExist(err) {
				return nil, fmt.Errorf("%s: prompt_file %q does not exist", evalID, r.PromptFile)
			}
		}

		if len(r.Assert) == 0 {
			return nil, fmt.Errorf("%s has empty assert: list", evalID)
		}

		assertions, err := parseAssertions(evalID, r.Assert)
		if err != nil {
			return nil, err
		}

		var models *ModelsBlock
		if r.Models != nil {
			if r.Models.Primary == "" {
				return nil, fmt.Errorf("%s has `models:` without `models.primary:`. Primary is required.", evalID)
			}
			if r.Models.Secondaries != nil && len(r.Models.Secondaries) == 0 {
				return nil, fmt.Errorf("%s has `models.secondaries: []`. Either omit the field or provide entries.", evalID)
			}
			models = &ModelsBlock{
				Primary:     r.Models.Primary,
				Secondaries: r.Models.Secondaries,
			}
		}

		evals = append(evals, Eval{
			ID:         evalID,
			Tests:      r.Tests,
			Prompt:     r.Prompt,
			PromptFile: r.PromptFile,
			Input:      r.Input,
			Assert:     assertions,
			Models:     models,
		})
	}

	return evals, nil
}

func parseAssertions(evalID string, raws []map[string]string) ([]Assertion, error) {
	assertions := make([]Assertion, 0, len(raws))
	for _, raw := range raws {
		if len(raw) != 1 {
			return nil, fmt.Errorf("%s: assertion must have exactly one key, got %d", evalID, len(raw))
		}
		var aType, aValue string
		for k, v := range raw {
			aType = k
			aValue = v
		}
		if !knownAssertionTypes[aType] {
			return nil, fmt.Errorf("%s has unknown assertion type %q", evalID, aType)
		}
		if aType == "matches" || aType == "not_matches" {
			if _, err := regexp.Compile(aValue); err != nil {
				return nil, fmt.Errorf("%s: assertion %s has invalid regex %q: %w", evalID, aType, aValue, err)
			}
		}
		assertions = append(assertions, Assertion{Type: aType, Value: aValue})
	}
	return assertions, nil
}
