package skilleval

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	substrateIDRe     = regexp.MustCompile(`^([A-Z]+-\d+)`)
	evalIDNumericRe   = regexp.MustCompile(`^EV-(\d+)$`)
	errMalformedEvals = errors.New("malformed evals file")
)

const defaultConfigContent = `default_model: claude-sonnet-4-6
evals_file: evals.yml
results_dir: evals/results
per_eval_timeout_seconds: 60
concurrency: 4
`

// createConfigIfMissing writes defaultConfigContent to path when the file does
// not already exist. Returns true if the file was created, false if it already
// existed, and an error on write failure.
func createConfigIfMissing(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, []byte(defaultConfigContent), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// aiGeneratedEntry holds the eval fields produced by generateEvalFields.
type aiGeneratedEntry struct {
	Input   string
	Asserts []map[string]string
}

// generateEvalFields reads the prompt file at promptFilePath, sends it to
// Claude with a meta-prompt requesting a realistic input task and assertions,
// and returns the parsed fields. Returns an error if the file cannot be read,
// the Claude call fails, or the response cannot be parsed.
func generateEvalFields(promptFilePath, model string, timeoutSeconds int) (aiGeneratedEntry, error) {
	content, err := os.ReadFile(promptFilePath)
	if err != nil {
		return aiGeneratedEntry{}, fmt.Errorf("reading prompt file: %w", err)
	}

	metaPrompt := "You are generating a test case for a prompt fragment used with an AI assistant.\n\n" +
		"The prompt fragment is prepended to a user task before the model sees it.\n" +
		"Suggest a realistic input task and two or three assertions about what correct output should contain.\n\n" +
		"Prompt fragment:\n---\n" + string(content) + "\n---\n\n" +
		"Respond with ONLY valid YAML in this exact format — no explanation, no markdown fences:\n\n" +
		"input: \"a realistic task that exercises this prompt\"\n" +
		"assert:\n" +
		"  - contains: \"text the output should contain\"\n" +
		"  - contains: \"another expected substring\"\n"

	result, err := RunClaude(metaPrompt, model, timeoutSeconds)
	if err != nil {
		return aiGeneratedEntry{}, fmt.Errorf("claude invocation: %w", err)
	}

	var parsed struct {
		Input  string              `yaml:"input"`
		Assert []map[string]string `yaml:"assert"`
	}
	if err := yaml.Unmarshal([]byte(result.Output), &parsed); err != nil {
		return aiGeneratedEntry{}, fmt.Errorf("parsing AI response: %w", err)
	}
	if parsed.Input == "" || len(parsed.Assert) == 0 {
		return aiGeneratedEntry{}, fmt.Errorf("AI response missing required fields")
	}

	return aiGeneratedEntry{Input: parsed.Input, Asserts: parsed.Assert}, nil
}

// formatAIEntry returns YAML text for an eval entry with AI-generated fields.
// A review comment is included to remind the user to validate before running.
func formatAIEntry(id, substrateID, promptFile string, fields aiGeneratedEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- id: %s\n", id)
	fmt.Fprintf(&sb, "  tests: %s\n", substrateID)
	fmt.Fprintf(&sb, "  prompt_file: %s\n", promptFile)
	sb.WriteString("  # AI-generated — review input and assertions before running\n")
	fmt.Fprintf(&sb, "  input: %q\n", fields.Input)
	sb.WriteString("  assert:\n")
	for _, a := range fields.Asserts {
		for k, v := range a {
			fmt.Fprintf(&sb, "    - %s: %q\n", k, v)
		}
	}
	return sb.String()
}

// loadAIConfig returns the model and timeout to use for AI generation.
// Falls back to sensible defaults if the config file is absent or invalid.
func loadAIConfig(configPath string) (model string, timeoutSeconds int) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return "claude-sonnet-4-6", 60
	}
	return cfg.DefaultModel, cfg.PerEvalTimeoutSeconds
}

// ExtractSubstrateID extracts the prompt ID from a file path.
// If the basename matches [A-Z]+-\d+, that prefix is returned (e.g. RU-001).
// Otherwise, the filename stem (without extension) is used as-is.
func ExtractSubstrateID(path string) string {
	base := filepath.Base(path)
	if m := substrateIDRe.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

// evalInitEntry holds the init-relevant data extracted from one eval entry.
type evalInitEntry struct {
	ID    string
	Tests string
	Line  int // source line in evals.yml where this mapping begins
}

// parseEvalsForInit reads the evals file and returns minimal per-entry data
// with source line numbers. Uses the yaml.v3 Node API to preserve line info.
// Returns nil, nil when the file does not exist or is empty.
// Returns nil, errMalformedEvals when the file cannot be parsed as a YAML list.
func parseEvalsForInit(path string) ([]evalInitEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, errMalformedEvals
	}

	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil
	}

	root := doc.Content[0]
	// Null/empty document parses as a scalar "null".
	if root.Kind == yaml.ScalarNode {
		return nil, nil
	}
	if root.Kind != yaml.SequenceNode {
		return nil, errMalformedEvals
	}

	entries := make([]evalInitEntry, 0, len(root.Content))
	for _, item := range root.Content {
		if item.Kind != yaml.MappingNode {
			return nil, errMalformedEvals
		}
		var id, tests string
		for i := 0; i+1 < len(item.Content); i += 2 {
			switch item.Content[i].Value {
			case "id":
				id = item.Content[i+1].Value
			case "tests":
				tests = item.Content[i+1].Value
			}
		}
		entries = append(entries, evalInitEntry{ID: id, Tests: tests, Line: item.Line})
	}
	return entries, nil
}

// nextEvalID computes the next available EV-NNN identifier from the given entries.
// If no EV-NNN IDs are found, returns EV-001.
func nextEvalID(entries []evalInitEntry) string {
	maxN := 0
	for _, e := range entries {
		m := evalIDNumericRe.FindStringSubmatch(e.ID)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > maxN {
			maxN = n
		}
	}
	return fmt.Sprintf("EV-%03d", maxN+1)
}

// formatPlaceholderEntry returns the YAML text for a new placeholder eval entry.
// The entry includes TODO comments for fields the user must supply.
func formatPlaceholderEntry(id, substrateID, promptFile string) string {
	return fmt.Sprintf(
		"- id: %s\n"+
			"  tests: %s\n"+
			"  prompt_file: %s\n"+
			"  # TODO: describe the task the model should perform\n"+
			"  input: \"\"\n"+
			"  assert:\n"+
			"    # TODO: define assertions for what the output should contain\n"+
			"    - contains: \"\"\n",
		id, substrateID, promptFile,
	)
}

// appendToEvalsFile appends entry to the file at path using raw byte manipulation
// so existing comments and formatting are preserved.
// Creates the file when it does not exist or is empty.
// Ensures a newline separator before the new entry when appending.
func appendToEvalsFile(path, entry string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var buf []byte
	if len(existing) == 0 {
		buf = []byte(entry)
	} else {
		if existing[len(existing)-1] != '\n' {
			existing = append(existing, '\n')
		}
		buf = append(existing, []byte(entry)...)
	}

	return os.WriteFile(path, buf, 0644)
}

// initEvalsFile returns the configured evals file path from the config file,
// falling back to "evals.yml" if the config is missing or lacks an evals_file entry.
func initEvalsFile(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "evals.yml"
	}
	var c struct {
		EvalsFile string `yaml:"evals_file"`
	}
	if err := yaml.Unmarshal(data, &c); err != nil || c.EvalsFile == "" {
		return "evals.yml"
	}
	return c.EvalsFile
}

// RunInit executes the init subcommand. It scaffolds a new placeholder eval entry
// in evals.yml for the substrate file identified by --path.
// Returns 0 on success, 2 on validation or I/O error.
func RunInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	pathFlag := fs.String("path", "", "path to the prompt file")
	dryRun := fs.Bool("dry-run", false, "print the entry without modifying evals.yml")
	configPath := fs.String("config", ".skill-eval.yml", "path to .skill-eval.yml config file")
	aiFlag := fs.Bool("ai", false, "generate input and assertions using Claude (review before running)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *pathFlag == "" {
		fmt.Fprintf(os.Stderr,
			"Error: skill-eval init requires --path PATH\n"+
				"Example: skill-eval init --path prompts/RU-001-params-expect.md\n",
		)
		return 2
	}

	if !*dryRun {
		created, err := createConfigIfMissing(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to create %s: %v\n", *configPath, err)
			return 2
		}
		if created {
			fmt.Printf("Created %s\n", *configPath)
		}
	}

	substrateID := ExtractSubstrateID(*pathFlag)

	evalsFile := initEvalsFile(*configPath)

	entries, parseErr := parseEvalsForInit(evalsFile)
	if parseErr == errMalformedEvals {
		fmt.Fprintf(os.Stderr,
			"Error: %s is malformed and cannot be parsed.\nFix the file before running init.\n",
			evalsFile,
		)
		return 2
	}
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "Error: reading %s: %v\n", evalsFile, parseErr)
		return 2
	}

	// Collect matching evals for the substrate.
	type matchRef struct {
		id   string
		line int
	}
	var matches []matchRef
	for _, e := range entries {
		if e.Tests == substrateID {
			matches = append(matches, matchRef{id: e.ID, line: e.Line})
		}
	}

	fmt.Printf("\n")

	if len(matches) > 0 {
		noun := "evals"
		if len(matches) == 1 {
			noun = "eval"
		}
		fmt.Printf("Found %d existing %s for %s:\n", len(matches), noun, substrateID)
		for _, m := range matches {
			fmt.Printf("  %s (line %d)\n", m.id, m.line)
		}
		fmt.Printf("\n")
	}

	nextID := nextEvalID(entries)

	var entry string
	if *aiFlag {
		fmt.Printf("Generating eval fields for %s...\n\n", *pathFlag)
		model, timeout := loadAIConfig(*configPath)
		fields, err := generateEvalFields(*pathFlag, model, timeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: AI generation failed (%v); using placeholder instead.\n\n", err)
			entry = formatPlaceholderEntry(nextID, substrateID, *pathFlag)
		} else {
			entry = formatAIEntry(nextID, substrateID, *pathFlag, fields)
		}
	} else {
		entry = formatPlaceholderEntry(nextID, substrateID, *pathFlag)
	}

	if *dryRun {
		fmt.Printf("Would add %s to %s (--dry-run, no changes made).\n", nextID, evalsFile)
	} else {
		if err := appendToEvalsFile(evalsFile, entry); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to write %s: %v\n", evalsFile, err)
			return 2
		}
		fmt.Printf("Added %s to %s.\n", nextID, evalsFile)
	}

	fmt.Printf("\n")
	for _, line := range strings.Split(strings.TrimRight(entry, "\n"), "\n") {
		fmt.Printf("  %s\n", line)
	}

	return 0
}
