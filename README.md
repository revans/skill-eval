# skill-eval

A Go CLI for measuring the impact of a prompt fragment on Claude's output.

## What the tool does

Given an eval that defines a prompt fragment and an input task, `skill-eval` runs the input with the prompt loaded and checks the model output against assertions. Per-eval results are written to `evals/results/{tests-id}/{eval-id}/`. A summary YAML is automatically written to `evals/summaries/` after every run.

With `--compare`, each eval runs twice. The first run sends the prompt + input. The second sends only the input — the model sees the task with no prompt context. The tool classifies the prompt's effect based on which runs passed.

## Prompt vs input distinction

- **`prompt`** (or `prompt_file`) — the fragment being tested. Toggled on and off in compare mode.
- **`input`** — the task the model performs. Constant across all runs.

Without `--compare`, the runner sends `{prompt}\n\n{input}` to Claude. With `--compare`, it also sends the bare `{input}` as a second, independent invocation.

## Prerequisites

- **Go 1.22+** — [install](https://go.dev/dl/)
- **Claude CLI** — `skill-eval` shells out to `claude -p` for every eval. Install it and authenticate before running:

```bash
npm install -g @anthropic-ai/claude-code
claude                      # follow the login prompt to authenticate
claude -p "hello" --model claude-sonnet-4-6   # verify it works
```

The Claude CLI must be on your `$PATH`. The `default_model` in `.skill-eval.yml` must be a model your account can access.

## Installation

```bash
cd skill-eval
go build -o skill-eval ./cmd/skill-eval
```

Or install to `$GOPATH/bin`:

```bash
go install github.com/revans/skill-eval/cmd/skill-eval@latest
```

## Quick start

### `.skill-eval.yml`

```yaml
default_model: claude-sonnet-4-6
evals_file: evals.yml
results_dir: evals/results
per_eval_timeout_seconds: 60
concurrency: 4
```

### `evals.yml`

```yaml
- id: EV-001
  tests: RU-001
  prompt: "When writing Rails controller code, use params.expect for mass assignment protection."
  input: "Write a Rails controller create action for a User model with name and email attributes."
  assert:
    - contains: "params.expect"
    - not_contains: "params.permit"

- id: EV-002
  tests: RU-001
  prompt_file: substrate/rules/RU-001-params-expect.md
  input: "Write a Rails controller update action for a Post model with title and body."
  assert:
    - contains: "params.expect"
    - matches: "def (create|update)"
```

### Run it

```bash
skill-eval --version              # print version and exit
skill-eval                        # sequential, single-run mode
skill-eval --concurrency 4        # 4 parallel workers
skill-eval --compare              # compare mode: two runs per eval, classified
skill-eval --compare --concurrency 4
skill-eval --filter RU-001        # filter by tests: field
skill-eval --filter EV-00         # filter by ID prefix

# Targeted mode — focused compare for a specific prompt file or eval ID
skill-eval --prompt-file substrate/rules/RU-001-params-expect.md
skill-eval --eval EV-007
skill-eval --prompt-file substrate/rules/RU-001-params-expect.md --concurrency 4

# Multi-model targeted mode — compare across models with matrix output
skill-eval --eval EV-007 --model claude-sonnet-4-6,claude-haiku-4-5
skill-eval --eval EV-007 --all-models              # uses eval's models: block
skill-eval --prompt-file substrate/rules/RU-001-params-expect.md --model claude-sonnet-4-6,claude-haiku-4-5

# Scaffold a new eval entry for a prompt file
skill-eval init --path substrate/rules/RU-001-params-expect.md
skill-eval init --path substrate/rules/RU-001-params-expect.md --dry-run

# Scan a directory or glob for prompt files and scaffold entries for all of them
skill-eval scan --dir prompts/
skill-eval scan --glob "rules/*.md"
skill-eval scan --dir prompts/ --glob "extra/*.md"
skill-eval scan --dir prompts/ --dry-run

# Compile a summary from historical per-eval artifacts (e.g. after a crash)
skill-eval compile-summary --timestamp 2026-05-02-T14-23

# Compile an aggregate summary spanning multiple runs
skill-eval compile-summary --since 2026-05-01-T00-00 --until 2026-05-02-T23-59
```

Example output (single mode, with concurrency):

```
Running 4 evals against claude-sonnet-4-6 (concurrency: 4)...

[1/4] PASS  EV-001    RU-001   (2.3s)
[2/4] PASS  EV-002    RU-001   (2.1s)
[3/4] FAIL  EV-003    RU-013   (2.4s)
      contains "includes(" - failed
[4/4] PASS  EV-004    RU-002   (1.8s)

Run complete: 3 passed, 1 failed in 2.5s.

Failures:
  EV-003    RU-013    contains "includes(" - failed
```

Example output (compare mode):

```
Running 4 evals against claude-sonnet-4-6 in compare mode (concurrency: 4)...

[1/4] LOAD-BEARING  EV-001    RU-001   (4.2s)
[2/4] OBSOLETE      EV-002    RU-001   (4.1s)
[3/4] INSUFFICIENT  EV-003    RU-013   (4.4s)
[4/4] HARMFUL       EV-019    RU-021   (4.0s)

Run complete: 4 evals classified in 8.4s.

Classifications:
  Load-bearing:    1    (25%)
  Obsolete:        1    (25%)
  Insufficient:    1    (25%)
  Harmful:         1    (25%)
  Errors:          0

Notable findings:
  Obsolete (consider removal):
    RU-001    EV-002
  Harmful (investigate):
    RU-021    EV-019
```

## Summary files

After every suite run, `skill-eval` writes a summary YAML to `evals/summaries/{timestamp}.yml`. The file captures the complete run outcome in one place — pass/fail counts for single mode, classification counts for compare mode — without requiring you to open individual per-eval files.

Summaries accumulate forever. They are the longitudinal record of your prompt suite's health. Use git to track them:

```bash
git add evals/summaries/
git commit -m "Run evals 2026-05-02"
git log evals/summaries/       # history of every run
git diff HEAD~1 -- evals/summaries/  # what changed since last run
```

Example single-mode summary:

```yaml
ran_at: 2026-05-02T14:23:00Z
model: claude-sonnet-4-6
mode: single
total_evals: 247
total_duration_seconds: 1114
total_eval_time_seconds: 4253
total_passed: 245
total_failed: 2
results:
  - eval_id: EV-001
    tests: RU-001
    status: pass
    duration_ms: 2341
  - eval_id: EV-003
    tests: RU-013
    status: fail
    duration_ms: 2402
    failure_reason: 'contains "includes(" - failed'
```

Example compare-mode summary:

```yaml
ran_at: 2026-05-02T14:23:00Z
model: claude-sonnet-4-6
mode: compare
total_evals: 247
total_duration_seconds: 1114
total_eval_time_seconds: 8506
classifications:
  load-bearing: 198
  obsolete: 12
  insufficient: 35
  harmful: 2
  error: 0
results:
  - eval_id: EV-001
    tests: RU-001
    classification: load-bearing
    duration_ms: 4234
```

`total_duration_seconds` is wall-clock time. `total_eval_time_seconds` is the sum of per-eval durations. With concurrency, wall clock is typically much less than eval-time-sum.

### `compile-summary` subcommand

Assembles a summary from per-eval YAML files already on disk. Useful after a crash (where the auto-write never ran) or to build aggregate summaries spanning multiple runs.

```bash
# Recover summary for a specific run (all evals that wrote artifacts at this minute)
skill-eval compile-summary --timestamp 2026-05-02-T14-23

# Aggregate all evals run within a date range
skill-eval compile-summary --since 2026-05-01-T00-00 --until 2026-05-02-T23-59
```

The `--timestamp` form writes to `evals/summaries/{timestamp}.yml`. The `--since/--until` form writes to `evals/summaries/aggregate-{since}-to-{until}.yml`.

Note: `total_duration_seconds` is 0 in compiled summaries — the original wall-clock time is not recorded in per-eval artifacts.

## Compare-mode classifications

Each eval receives one of four classifications based on which runs passed assertions:

| Classification | With prompt | Without prompt | Meaning |
|---------------|-------------|----------------|---------|
| `load-bearing` | pass | fail | Prompt is doing real work — model needs it |
| `obsolete` | pass | pass | Model no longer needs the prompt; consider removing |
| `insufficient` | fail | fail | Neither run works; investigate the prompt or the eval |
| `harmful` | fail | pass | Prompt degrades output; investigate immediately |

Compare mode is a **survey instrument**, not a CI gate. The exit code reflects whether the tool ran successfully (exit 1 = subprocess errors), not the distribution of classifications. Use single mode (`--compare` absent) for CI regression.

Compare mode roughly doubles runtime because each eval makes two `claude -p` subprocess calls. The two calls within an eval are sequential (not parallel) by design. Concurrency across evals still applies.

## Targeted mode

`--prompt-file PATH` and `--eval ID` switch the tool into targeted mode. Targeted mode:

- Is always compare mode — runs each matched eval twice and classifies the result.
- Prints a detailed per-assertion breakdown to stdout, including ✓/✗ per assertion and a classification explanation.
- Never writes a summary file to `evals/summaries/` — it is a development tool, not a CI instrument.
- Still writes per-eval artifacts to `evals/results/{tests-id}/{eval-id}/`.

`--prompt-file` runs all evals whose `prompt_file:` field exactly matches the given path. Use it during the prompt TDD loop: edit the prompt file, run `skill-eval --prompt-file <path>`, check the classification.

`--eval` runs a single eval by its exact ID.

`--filter` and `--compare` cannot be combined with `--prompt-file` or `--eval` (targeted mode is always compare; filtering is implicit).

Example output (single eval):

```
Running EV-001 against RU-001 in compare mode...
Model: claude-sonnet-4-6

WITH prompt:
  contains "params.expect"              ✓
  not_contains "params.permit"          ✓
  Result: PASS (2.3s)

WITHOUT prompt:
  contains "params.expect"              ✗
  not_contains "params.permit"          ✓
  Result: FAIL (2.1s)

Classification: LOAD-BEARING

The prompt is doing work. The model needs the prompt loaded to produce
correct output for this eval.

Artifacts: evals/results/RU-001/EV-001
```

Example output (multiple evals via `--prompt-file`):

```
Running 3 evals against RU-001 in compare mode...
Model: claude-sonnet-4-6

EV-001    LOAD-BEARING  (4.4s)
EV-002    OBSOLETE      (4.1s)
EV-003    LOAD-BEARING  (4.5s)

Summary:
  Load-bearing:    2
  Obsolete:        1

Notes:
  EV-002 obsolete — model produces correct output without the prompt.

Artifacts: evals/results/RU-001
```

### Multi-model matrix output

When two or more models are specified, targeted mode renders a matrix table:

```
$ skill-eval --prompt-file substrate/rules/RU-001-params-expect.md --model claude-sonnet-4-6,claude-haiku-4-5,claude-opus-4-7

Running 3 evals against RU-001 in compare mode across 3 models...

                        Sonnet      Haiku       Opus        
RU-001 (EV-001)         LOAD        LOAD        LOAD        (sonnet 4.2s, haiku 3.1s, opus 4.8s)
RU-001 (EV-002)         OBSOLETE    LOAD        LOAD        (sonnet 4.0s, haiku 2.9s, opus 4.6s)
RU-001 (EV-003)         LOAD        HARMFUL     LOAD        (sonnet 4.3s, haiku 3.2s, opus 4.9s)

Per-model summary:
  claude-sonnet-4-6: 2 load-bearing, 1 obsolete
  claude-haiku-4-5: 2 load-bearing, 1 harmful
  claude-opus-4-7: 3 load-bearing

Notes:
  EV-002 has conflicting classifications:
    - Sonnet: OBSOLETE
    - Haiku: LOAD-BEARING
    - Opus: LOAD-BEARING
  This prompt is load-bearing on some models and obsolete on others.
  Consider whether the prompt should be model-conditional.
  EV-003 has conflicting classifications:
    - Sonnet: LOAD-BEARING
    - Haiku: HARMFUL
    - Opus: LOAD-BEARING
  This prompt is required on some models but degrades output on others.
  Consider tier-conditional deployment or rewrite.

Artifacts: evals/results/RU-001
```

**Column widths:** eval identifier is left-aligned in a 24-char column; each model column is 12 chars wide. Short model names are extracted from the model ID — the last all-alpha segment, excluding "claude", capitalized (`claude-sonnet-4-6` → `Sonnet`). Timing appears at the end of each data row as `(shortname Ns, ...)`. The per-model summary uses the full model identifier and lists only the classifications that appeared.

**Conflict detection** fires when:
- Any model classifies an eval as `load-bearing` while another classifies it `harmful` or `obsolete`
- Any model classifies an eval as `insufficient` while any other model does not

Conflicts appear in the `Notes:` section at the bottom. Runs where all models agree produce no Notes section.

### Multi-model artifact layout

Per-model runs write artifacts into model-named subdirectories:

```
evals/results/{tests-id}/{eval-id}/
  claude-sonnet-4-6/
    {timestamp}-with-prompt.md
    {timestamp}-without-prompt.md
    {timestamp}-result.yml
  claude-haiku-4-5/
    {timestamp}-with-prompt.md
    {timestamp}-without-prompt.md
    {timestamp}-result.yml
```

The per-model layout is **sticky**: once an eval directory contains model subdirs, subsequent single-model targeted runs also write into a model subdir rather than flat. This prevents flat files from mixing with per-model files.

When transitioning from a previous flat run to a multi-model run, flat files at the eval root are removed and model subdirs are created. Other model subdirs from prior runs are preserved.

### Prompt TDD loop

```bash
# 1. Write or edit the prompt file
$EDITOR substrate/rules/RU-001-params-expect.md

# 2. Run targeted eval
skill-eval --prompt-file substrate/rules/RU-001-params-expect.md

# 3. Inspect classification and assertion breakdown
# 4. Repeat until load-bearing
```

### Multi-model comparison workflow

```bash
# 1. Confirm behavior against the primary model first
skill-eval --eval EV-007

# 2. Compare across all declared models
skill-eval --eval EV-007 --all-models

# 3. Check for conflicts in the Notes section — investigate HARMFUL or diverging results
# 4. Promote to suite mode once load-bearing across all models
```

### Running suite mode against multiple models (shell loop)

`--model` and `--all-models` are not available in suite mode. To run the full suite against multiple models, loop in your shell:

```bash
for model in claude-sonnet-4-6 claude-haiku-4-5 claude-opus-4-7; do
  echo "=== $model ==="
  skill-eval --compare
done
```

## Recipes

### Basic regression check (suite mode, default model)

Run the whole suite in single mode. Exit code 1 means something failed.

```bash
skill-eval
echo "Exit: $?"
```

Use this in CI. The default model comes from `.skill-eval.yml`. A summary is written to `evals/summaries/`; each eval's result also lands in `evals/results/`.

### Quarterly health check (compare mode)

Every few months, run compare mode over the full suite to see which prompts the model has outgrown:

```bash
skill-eval --compare --concurrency 4
cat evals/summaries/$(ls -t evals/summaries/ | head -1)
```

Obsolete classifications signal prompts the model no longer needs. Harmful signals prompts actively degrading output. Neither forces any action — this is a survey, not a gate.

### TDD loop while developing a prompt (targeted, single model)

Edit your prompt file and immediately see whether it moved the needle:

```bash
$EDITOR substrate/rules/RU-001-params-expect.md

skill-eval --prompt-file substrate/rules/RU-001-params-expect.md
# ↑ shows per-assertion ✓/✗ and classification explanation

# Keep iterating until you see: Classification: LOAD-BEARING
```

Targeted mode never writes a summary — it's a tight iteration loop, not a record-keeping run.

### Investigating a prompt across model tiers (targeted, `--all-models`)

Once a prompt is load-bearing on the primary model, check whether it holds across your declared secondaries:

```bash
# Requires the eval's models: block to be populated:
#   models:
#     primary: claude-sonnet-4-6
#     secondaries:
#       - claude-haiku-4-5
#       - claude-opus-4-7

skill-eval --eval EV-001 --all-models
```

The matrix output flags cross-model conflicts. A load-bearing on Sonnet but harmful on Haiku means tier-conditional deployment or a prompt rewrite is needed before shipping to both tiers.

### Pre-deployment validation for a new model release (suite mode, shell loop)

When a new model becomes available, run the full suite against it before switching the config default. Because suite mode reads the model from the config file, use per-model configs or a `sed` substitution:

```bash
for model in claude-sonnet-4-6 claude-haiku-4-5 claude-opus-4-7; do
  echo "=== $model ==="
  sed "s/^default_model:.*/default_model: $model/" .skill-eval.yml > /tmp/.skill-eval-$model.yml
  skill-eval --config /tmp/.skill-eval-$model.yml --compare --concurrency 4
done
```

Each iteration writes its own summary file (different timestamp — one per model). Review the summaries to spot regressions before promoting the new model to `default_model` in `.skill-eval.yml`.

## Flag reference

### Suite mode (`skill-eval [flags]`)

| Flag | Default | Description |
|------|---------|-------------|
| `--compare` | false | Run each eval twice and classify the prompt's effect |
| `--concurrency N` | from config, fallback 1 | Number of parallel eval workers |
| `--filter <expr>` | — | Run only evals matching this ID prefix or `tests:` value |
| `--config <path>` | `.skill-eval.yml` | Path to config file |

### Targeted mode (`skill-eval --prompt-file PATH` or `skill-eval --eval ID`)

| Flag | Description |
|------|-------------|
| `--prompt-file <path>` | Run all evals whose `prompt_file:` matches this path (compare, no summary) |
| `--eval <id>` | Run the single eval with this exact ID (compare, no summary) |
| `--model M1,M2,...` | Run against these comma-separated models instead of the config default |
| `--all-models` | Run against the eval's `models.primary` + all `models.secondaries` |
| `--concurrency N` | Number of parallel workers; total cap across all models (default: from config) |
| `--config <path>` | Path to config file (default: `.skill-eval.yml`) |

`--model` and `--all-models` are mutually exclusive. Both are rejected in suite mode with an error that includes a shell-loop alternative.

**Model resolution order (four tiers):**

1. `--model` flag — explicit override, wins over everything
2. `--all-models` — expands to `models.primary` + `models.secondaries` from the eval's `models:` block (error if no block)
3. Eval's `models.primary` — used when no flag is set but the eval declares a preferred model
4. Config `default_model` — fallback when the eval has no `models:` block

### init subcommand (`skill-eval init --path PATH [--dry-run]`)

| Flag | Description |
|------|-------------|
| `--path <path>` | **Required.** Path to the prompt file. |
| `--dry-run` | Print what would be added without modifying `evals.yml` |
| `--config <path>` | Path to config file (default: `.skill-eval.yml`) |

`--path` is required.

### scan subcommand (`skill-eval scan [flags]`)

| Flag | Description |
|------|-------------|
| `--dir <path>` | Scan all `.md` files in this directory (recursive). |
| `--glob <pattern>` | Scan files matching this glob pattern. |
| `--dry-run` | Print what would be added without modifying `evals.yml`. |
| `--config <path>` | Path to config file (default: `.skill-eval.yml`). |

At least one of `--dir` or `--glob` is required. Both can be combined — results are deduplicated.

`--glob` uses standard shell glob syntax (`*` matches within a directory). For recursive scans, use `--dir`. For cross-directory selections, combine both flags or run `scan` twice.

Files that already have an eval (matched by the `tests:` value derived from their filename) are skipped. New entries are appended to `evals.yml` with TODO placeholders — fill in `input:` and `assert:` before running evals.

Example output:

```
$ skill-eval scan --dir prompts/

Adding 3 entries to evals.yml:

  EV-001    my-code-review            prompts/my-code-review.md
  EV-002    RU-001                    prompts/RU-001-params-expect.md
  EV-003    summarizer                prompts/summarizer.md

Done. Fill in the TODO fields in evals.yml before running evals.
```

### compile-summary subcommand (`skill-eval compile-summary [flags]`)

| Flag | Description |
|------|-------------|
| `--timestamp <ts>` | Compile summary for evals at exactly this timestamp (`YYYY-MM-DD-THH-MM`) |
| `--since <ts>` | Lower bound for range compile (inclusive) |
| `--until <ts>` | Upper bound for range compile (inclusive) |
| `--config <path>` | Path to config file (default: `.skill-eval.yml`) |

Provide either `--timestamp` or both `--since` and `--until`.

## Concurrency considerations

Each worker starts its own `claude -p` subprocess. In compare mode, each worker makes two subprocess calls per eval. With `--concurrency 4 --compare`, up to 8 `claude` processes run simultaneously.

At high concurrency (10+):
- API rate limits may throttle requests, causing increased per-eval latency or errors (one retry with a 2-second delay)
- The per-eval timeout applies independently to each subprocess call
- The tool does not enforce a maximum concurrency value

Start at `--concurrency 4` and increase if your API plan supports it.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All evals ran successfully (single: all passed; compare: all classified, no errors) |
| 1 | At least one eval failed assertions (single) or errored (compare) |
| 2 | Config or startup error: malformed YAML, missing `default_model`, invalid flags |

In compare mode, `harmful` and `insufficient` classifications are **not** failures — exit code 0 means the tool ran cleanly, regardless of what it found.

## Assertion types

| Type | Passes when |
|------|-------------|
| `contains: "text"` | Output includes the substring |
| `not_contains: "text"` | Output does not include the substring |
| `matches: "regex"` | Output matches the regular expression |
| `not_matches: "regex"` | Output does not match the regular expression |

All assertions in an eval's `assert:` list must pass (AND semantics).

**Tip on `not_contains`:** avoid matching common English words that appear in prose. `not_contains: "boolean"` fires if Claude writes "don't use a boolean column." Match the code form instead: `not_matches: 't\.boolean|add_column.*:boolean'`. Use single-quoted YAML strings for regex patterns containing backslashes.

## Artifact structure

Single mode writes:

```
evals/results/{tests-id}/{eval-id}/
  {timestamp}-with-prompt.md    # raw model output
  {timestamp}-result.yml        # structured result, mode: single
```

Compare mode writes:

```
evals/results/{tests-id}/{eval-id}/
  {timestamp}-with-prompt.md      # output from the with-prompt run
  {timestamp}-without-prompt.md   # output from the without-prompt run
  {timestamp}-result.yml          # structured result, mode: compare
```

The compare result YAML includes both run blocks and the classification:

```yaml
eval_id: EV-001
tests: RU-001
ran_at: 2026-05-02T14:23:00Z
mode: compare
model: claude-sonnet-4-6
prompt_source: file
prompt_file: substrate/rules/RU-001-params-expect.md
input: "Write a Rails controller create action."
with_prompt:
  output_file: 2026-05-02-T14-23-with-prompt.md
  duration_ms: 2341
  assertions:
    - type: contains
      value: "params.expect"
      result: pass
  status: pass
without_prompt:
  output_file: 2026-05-02-T14-23-without-prompt.md
  duration_ms: 1987
  assertions:
    - type: contains
      value: "params.expect"
      result: fail
  status: fail
classification: load-bearing
```

Timestamp format: `YYYY-MM-DD-THH-MM` (colons replaced with hyphens for filesystem compatibility).

## Eval YAML reference

```yaml
- id: EV-NNN              # Required. Unique identifier, EV-NNN format.
  tests: my-prompt        # Required. Any string identifying which prompt this eval covers.
  prompt: "..."           # Exactly one of prompt or prompt_file required.
  prompt_file: path/to/file.md
  input: "..."            # Required. The task the model performs.
  models:                 # Optional. Used by --all-models in targeted mode.
    primary: claude-sonnet-4-6
    secondaries:
      - claude-haiku-4-5
      - claude-opus-4-7
  assert:                 # Required. Non-empty list of assertions.
    - contains: "..."
    - not_contains: "..."
    - matches: "..."
    - not_matches: "..."
```

## init subcommand — eval scaffolding

`skill-eval init --path PATH` is a pure scaffolding operation. It does not run evals, invoke `claude -p`, or write to `evals/results/` or `evals/summaries/`. It only modifies `evals.yml`.

### What it does

1. Extracts the prompt ID from the filename (`RU-001-params-expect.md` → `RU-001`).
2. Reads `evals.yml`, reports any existing evals for that ID, and computes the next available `EV-NNN` ID.
3. Appends a placeholder entry with `TODO` comments for the fields you must fill in.

### Example

```
$ skill-eval init --path substrate/rules/RU-001-params-expect.md

Found 2 existing evals for RU-001:
  EV-022 (line 87)
  EV-023 (line 95)

Added EV-024 to evals.yml.

  - id: EV-024
    tests: RU-001
    prompt_file: substrate/rules/RU-001-params-expect.md
    # TODO: describe the task the model should perform
    input: ""
    assert:
      # TODO: define assertions for what the output should contain
      - contains: ""
```

### TODO workflow

After running `init`, open `evals.yml` and fill in the two TODO fields:

1. **`input:`** — the task the model should perform (e.g., `"Write a Rails controller create action for a User model."`).
2. **`assert:`** — replace the empty `contains: ""` with meaningful assertions (e.g., `contains: "params.expect"`).

The placeholder entry will fail if run before you fill in the TODOs — the empty `input:` fails validation and an empty `contains:` assertion tells you nothing. That is intentional.

### Dry-run mode

`--dry-run` shows what would be added without modifying `evals.yml`:

```
$ skill-eval init --path substrate/rules/RU-001-params-expect.md --dry-run

Would add EV-024 to evals.yml (--dry-run, no changes made).

  - id: EV-024
    ...
```

### Append-only, comment-safe

`init` appends raw text to `evals.yml` rather than parsing and rewriting. Existing comments, blank lines, and formatting choices in the file are preserved exactly.

### Prompt ID extraction

`init` derives the `tests:` value from the prompt file's basename. If the basename starts with an uppercase prefix followed by a number (`PREFIX-NUMBER`), that prefix is used as the ID. Otherwise, the full filename stem (without extension) is used.

| Filename | Extracted ID |
|----------|-------------|
| `RU-001-params-expect.md` | `RU-001` |
| `PA-002-eager-loading.md` | `PA-002` |
| `my-code-review.md` | `my-code-review` |
| `summarizer.md` | `summarizer` |

## scan subcommand — bulk eval scaffolding

`skill-eval scan` is `init` for an entire directory or glob. It finds prompt files, skips any already covered in `evals.yml`, and appends stub entries for the rest — all in one pass.

```bash
# Scaffold evals for all .md files under prompts/ (recursive)
skill-eval scan --dir prompts/

# Scaffold evals for a specific pattern
skill-eval scan --glob "rules/RU-*.md"

# Combine: scan a directory and also pull in files from another location
skill-eval scan --dir prompts/ --glob "extra/*.md"

# Preview what would be added without writing anything
skill-eval scan --dir prompts/ --dry-run
```

`scan` uses the same ID extraction and `evals.yml` append logic as `init`. A file is considered already covered when its extracted ID matches the `tests:` value of any existing eval — those files are skipped with a notice. IDs continue from the highest existing `EV-NNN` in the file.

After scanning, open `evals.yml` and fill in the `input:` and `assert:` fields for each new entry. The TODO placeholders will fail validation if run unchanged — that is intentional.

