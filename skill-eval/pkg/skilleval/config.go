// Package skilleval implements the core logic for the skill-eval CLI.
package skilleval

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the project-level configuration loaded from .skill-eval.yml.
type Config struct {
	DefaultModel          string `yaml:"default_model"`
	EvalsFile             string `yaml:"evals_file"`
	ResultsDir            string `yaml:"results_dir"`
	SummariesDir          string `yaml:"summaries_dir"`
	Concurrency           int    `yaml:"concurrency"`
	PerEvalTimeoutSeconds int    `yaml:"per_eval_timeout_seconds"`
}

// LoadConfig reads the config file at path and applies defaults for missing fields.
// Returns an error if the file exists but is malformed, or if default_model is absent.
func LoadConfig(path string) (Config, error) {
	cfg := Config{
		EvalsFile:             "evals.yml",
		ResultsDir:            "evals/results",
		SummariesDir:          "evals/summaries",
		Concurrency:           1,
		PerEvalTimeoutSeconds: 60,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("reading config file %q: %w", path, err)
		}
		// Config file is optional; missing file is not an error.
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parsing config file %q: %w", path, err)
		}
	}

	if cfg.DefaultModel == "" {
		return cfg, fmt.Errorf("default_model is required in .skill-eval.yml")
	}
	return cfg, nil
}
