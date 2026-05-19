# skill-eval — Specification

## Purpose

A Go CLI for measuring the impact of a prompt fragment on Claude's output. Given an eval that defines a prompt and an input task, the tool runs the input both with and without the prompt and compares the results. Classifications reveal whether the prompt is doing useful work.

The tool is general-purpose for prompt-impact testing. The most common use case is testing substrate skills (rules, capabilities, patterns, procedures) — but any prompt fragment can be tested: few-shot example sets, instruction styles, personas, anything.

The tool supports two modes: a suite mode for regression testing and a targeted mode for prompt development. The targeted mode enables an eval-first methodology — write the eval first, build the minimum prompt that satisfies it, refine without breaking it.

## Design Principles

1. **Operational vs investigative separation.** Suite mode is deterministic and runs against the configured default model. Targeted mode is flexible and supports model selection. Each mode has the right defaults for its context.

2. **Prompt and input are distinct.** The prompt is what's being tested. The input is the task the model performs. Compare mode toggles the prompt's presence; the input is constant in both runs.

3. **Compare mode as classification, not pass/fail.** Comparing with-prompt and without-prompt runs produces one of four classifications: load-bearing, obsolete, insufficient, harmful. The classification informs decisions, not CI gates.

4. **Per-eval YAML files compiled into a summary.** Each eval execution writes its own result file. The summary is derived from per-eval files. This pattern composes with concurrency and survives crashes.

5. **Artifacts on disk for human review.** Model output is written as plain markdown alongside structured YAML results. A human can audit any run by reading files in a directory.

6. **Cleanup that respects intent.** Per-eval directories are wiped before each run for evals being run. Summaries are never deleted. Other evals' artifacts are not touched.

## Architecture

### Single Go binary called `skill-eval`

The binary supports two invocation modes determined by which flags are present.

### Suite mode

Used by CI and for periodic substrate health checks.

```
skill-eval [--filter EXPR] [--compare] [--concurrency N]
```

- Always runs against the default model from `.skill-eval.yml`
- Does not accept `--model` or `--all-models` (errors if provided)
- Ignores eval-level model declarations (`models:` block in YAML)
- Produces per-eval artifacts and one summary file in `evals/summaries/`
- `--filter` selects a subset by `id` prefix or `tests:` field
- `--compare` enables compare mode (each eval runs twice, with and without prompt)
- Without `--compare`, evals run once with prompt loaded — pure regression check

### Targeted mode

Used by humans investigating specific prompts during development.

```
skill-eval --prompt-file PATH [--eval ID] [--model M1,M2,...] [--all-models] [--concurrency N]
skill-eval --eval ID [--model M1,M2,...] [--all-models] [--concurrency N]
```

- Always runs in compare mode (no flag needed)
- Accepts `--model` as comma-separated list (single or multiple), overrides everything
- Accepts `--all-models` to expand to the eval's `models.primary` plus `models.secondaries`
- Without either flag, uses the eval's `models.primary` if declared, falls back to config default
- Single model produces flat artifacts in eval directory
- Multiple models produces per-model subdirectories
- Prints classification result(s) directly to stdout
- Does not produce a summary file

### Init subcommand

Used to scaffold new eval entries in `evals.yml` for substrate items.

```
skill-eval init --path PATH [--dry-run]
```

- Reads the substrate file at PATH
- Extracts the substrate ID from the filename (e.g., `RU-001-params-expect.md` → `RU-001`)
- Computes the next available eval ID by reading the existing `evals.yml`
- Appends a placeholder eval entry to `evals.yml` referencing the substrate file
- Reports any existing evals that test the same substrate item
- With `--dry-run`, prints what would be added without modifying the file
- Does not run any evals; this is a pure scaffolding operation

The placeholder entry has the mechanical fields filled in (`id`, `tests`, `prompt_file`) and empty placeholders for fields the user must supply (`input`, `assert`).

### Mode dispatch

```
If subcommand is "init":                 init subcommand
If subcommand is "compile-summary":      compile-summary subcommand
If --prompt-file or --eval present:      targeted mode
Else:                                    suite mode
If suite mode and --model:               error with helpful message
If suite mode and --all-models:          error with helpful message
If targeted mode and --filter:           error (filter is suite-only)
If targeted mode and --compare:          error (targeted is always compare)
If --model and --all-models together:    error (mutually exclusive)
```

## Configuration

### `.skill-eval.yml`

Project-level configuration in repo root.

```yaml
default_model: claude-sonnet-4-6
evals_file: evals.yml
results_dir: evals/results
summaries_dir: evals/summaries
concurrency: 1
per_eval_timeout_seconds: 60
```

All fields except `default_model` have built-in defaults. The config file is optional, but `default_model` must be specified somewhere — either in the config file or via CLI flag in targeted mode.

### `evals.yml`

Single file containing all eval definitions. Sequential entries.

#### Required fields

- `id` — sequential, EV-NNN format
- `tests` — substrate item ID this eval tests (e.g., RU-001), used for grouping and filtering
- `input` — the task the model is asked to perform
- `assert` — list of assertions
- Exactly one of `prompt` or `prompt_file` (see below)

#### Optional fields

- `models` — block declaring primary and secondary models for targeted mode

#### Prompt fields (mutually exclusive, exactly one required)

- `prompt` — inline prompt fragment text
- `prompt_file` — path to a file containing the prompt fragment

If both are present, the runner errors at startup. If neither is present, the runner errors at startup.

#### Models block

```yaml
models:
  primary: claude-sonnet-4-6
  secondaries:
    - claude-haiku-4-5
    - claude-opus-4-7
```

- `models.primary` is required if `models:` is present
- `models.secondaries` is optional; if present, must be a non-empty list
- The block is entirely optional — evals can omit `models:` to fall back to config default

#### Examples

Inline prompt with full model declaration:

```yaml
- id: EV-001
  tests: RU-001
  prompt: "When writing Rails code, use params.expect for mass assignment protection."
  input: "Write a Rails controller create action for a User model with name and email attributes."
  models:
    primary: claude-sonnet-4-6
    secondaries:
      - claude-haiku-4-5
      - claude-opus-4-7
  assert:
    - contains: "params.expect"
    - not_contains: "params.permit"
```

File-based prompt with primary only:

```yaml
- id: EV-002
  tests: RU-001
  prompt_file: substrate/rules/RU-001-params-expect.md
  input: "Write a Rails controller update action for a Post model with title and body."
  models:
    primary: claude-sonnet-4-6
  assert:
    - contains: "params.expect"
```

Minimal eval, no model declaration:

```yaml
- id: EV-003
  tests: RU-013
  prompt_file: substrate/rules/RU-013-n-plus-one.md
  input: "Write a Rails index action for Post that displays each post's author."
  assert:
    - contains: "includes("
```

## Validation Rules

The runner validates `evals.yml` at startup. Failures produce specific error messages naming the offending eval ID and exit with code 2.

### Required field validation

- Missing `id`, `tests`, `input`, or `assert` → error
- Invalid YAML structure → error

### Prompt field validation

- Both `prompt:` and `prompt_file:` set → error: "EV-XXX has both `prompt:` and `prompt_file:` set. Specify exactly one."
- Neither `prompt:` nor `prompt_file:` set → error: "EV-XXX has neither `prompt:` nor `prompt_file:` set. Specify one."
- `prompt_file:` references a file that doesn't exist → error at startup (validated before run begins)

### Models block validation

- `models:` set without `models.primary:` → error: "EV-XXX has `models:` without `models.primary:`. Primary is required."
- `models.secondaries: []` (empty list) → error: "EV-XXX has `models.secondaries: []`. Either omit the field or provide entries."

### ID validation

- Duplicate eval IDs → error naming both occurrences

### Assertion validation

- Unknown assertion type → error naming the eval and the unknown type
- Empty `assert:` list → error (eval has nothing to verify)

## Assertion Vocabulary

Four assertion types cover most cases. Additional types can be added later if the eval corpus reveals gaps.

### `contains: <string>`

Output must include this substring. Pass if found.

### `not_contains: <string>`

Output must not include this substring. Pass if not found.

### `matches: <regex>`

Output must match this regex. Pass if any match found.

### `not_matches: <regex>`

Output must not match this regex. Pass if no match found.

All assertions in an eval's `assert` list must pass for the eval to pass. AND semantics, no OR.

## Prompt Construction

The eval's `prompt` (or contents of `prompt_file`) and `input` are combined for the with-prompt run. The without-prompt run uses just the input.

### With-prompt invocation

```
{prompt}

{input}
```

A blank line separates the prompt fragment from the input task. The prompt comes first because it sets context; the input is the request.

### Without-prompt invocation

```
{input}
```

The bare input. The model has no additional context. This is the control condition for measuring the prompt's impact.

The runner does not add any wrapper text, instruction, or framing. The prompt fragment and input go to `claude -p` exactly as configured.

## Compare Mode Classification

Each compare-mode eval produces one of four classifications:

- **LOAD-BEARING** — passed with prompt, failed without prompt. The prompt is doing work.
- **OBSOLETE** — passed both with and without prompt. The prompt is no longer needed; the model produces correct output without it.
- **INSUFFICIENT** — failed both with and without prompt. Either the eval is wrong, the prompt is broken, or the task is beyond what prompts can fix.
- **HARMFUL** — failed with prompt, passed without prompt. The prompt is making the model worse.

Classification is mechanical from the two pass/fail results. The runner computes it after both runs complete.

## Model Resolution

### Suite mode

Always uses `default_model` from `.skill-eval.yml`. Eval-level `models:` declarations are ignored entirely.

### Targeted mode

Resolution order, highest priority first:

1. `--model M1,M2,...` flag → use exactly that list
2. `--all-models` flag → use eval's `models.primary` plus all `models.secondaries` (errors if eval has no `models:` block)
3. Eval's `models.primary` if declared → use that single model
4. Config's `default_model` → use that single model

Four-tier fallback. Each tier has unambiguous semantics.

### `--all-models` with no eval-level models

```
Error: --all-models requires the eval to declare a models: block.
EV-XXX has no models declared. Use --model to specify explicitly, 
or omit --all-models to use the default model.
```

## Concurrency Model

`--concurrency N` flag, defaults to 1.

- Worker pool of N goroutines
- Queue contains `(eval, model)` pairs in multi-model runs (one task per pair)
- Workers pull pairs, execute, write artifacts, return
- N is a global cap regardless of how many models are in scope
- Per-eval timeout configurable via `.skill-eval.yml` (default 60s)
- Single retry on transient `claude -p` failures (subprocess errors, timeouts)

The two runs within a single compare-mode eval (with-prompt and without-prompt) execute sequentially within one worker. They are not parallelized further.

## Artifact Structure

### Suite mode

```
evals/results/{tested-id}/{eval-id}/
  {timestamp}-with-prompt.md
  {timestamp}-without-prompt.md     # only if --compare
  {timestamp}-result.yml

evals/summaries/
  {timestamp}.yml
```

Timestamp format: `YYYY-MM-DD-THH-MM` (colons replaced with hyphens for filesystem compatibility).

### Targeted mode, single model

Same as suite mode for per-eval artifacts. No summary file.

### Targeted mode, multiple models

```
evals/results/{tested-id}/{eval-id}/
  {model-1}/
    {timestamp}-with-prompt.md
    {timestamp}-without-prompt.md
    {timestamp}-result.yml
  {model-2}/
    {timestamp}-with-prompt.md
    {timestamp}-without-prompt.md
    {timestamp}-result.yml
```

### Per-eval result YAML

Compare-mode form:

```yaml
eval_id: EV-001
tests: RU-001
ran_at: 2026-05-02T14:23:00Z
mode: compare
model: claude-sonnet-4-6
prompt_source: file
prompt_file: substrate/rules/RU-001-params-expect.md
input: "Write a Rails controller create action for a User model with name and email attributes."
with_prompt:
  output_file: 2026-05-02-T14-23-with-prompt.md
  duration_ms: 2341
  assertions:
    - type: contains
      value: "params.expect"
      result: pass
    - type: not_contains
      value: "params.permit"
      result: pass
  status: pass
without_prompt:
  output_file: 2026-05-02-T14-23-without-prompt.md
  duration_ms: 1987
  assertions:
    - type: contains
      value: "params.expect"
      result: fail
    - type: not_contains
      value: "params.permit"
      result: pass
  status: fail
classification: load-bearing
```

For inline prompts, `prompt_source: inline` and the `prompt:` field contains the full text instead of `prompt_file:`.

For non-compare runs, only `with_prompt` is present and `classification` is omitted.

### Summary YAML

For non-compare runs:

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
  - eval_id: EV-002
    tests: RU-001
    status: pass
    duration_ms: 2105
  - eval_id: EV-003
    tests: RU-013
    status: fail
    duration_ms: 2402
    failure_reason: 'contains "includes(" - failed'
```

For compare-mode runs:

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
  - eval_id: EV-002
    tests: RU-001
    classification: load-bearing
    duration_ms: 4087
```

Results are sorted by eval ID. `total_duration_seconds` is wall-clock time. `total_eval_time_seconds` is the sum of per-eval durations.

## Cleanup Rules

### On startup

For each eval in the run set, wipe the appropriate per-eval directory:

- Single-model run with no existing per-model subdirectories → wipe flat directory contents
- Single-model run with existing per-model subdirectories → wipe only the matching model's subdirectory; leave others
- Multi-model run → for each model in this run, wipe `evals/results/{tested}/{eval}/{model}/`; leave other models' subdirectories untouched

### Mixed-state handling

If a multi-model run encounters a directory with flat files (from a previous single-model run), wipe the flat files before creating per-model subdirectories. The runner's first action when starting a multi-model run is "normalize this directory to per-model layout."

If a single-model run encounters per-model subdirectories from a previous multi-model run, write to the matching subdirectory rather than creating flat files. This keeps the layout consistent once it's been used.

### What's never deleted

- `evals/summaries/` — append-only history
- Other evals' results — only the evals being run get their directories wiped
- Other models' results in per-model layout — only the models being run get wiped

## Output Formats

### Suite mode console output (single mode, no compare)

```
$ skill-eval

Running 247 evals against claude-sonnet-4-6...

PASS  EV-001  RU-001 (2.3s)
PASS  EV-002  RU-001 (2.1s)
FAIL  EV-003  RU-013 (2.4s)
      contains "includes(" - failed
PASS  EV-004  RU-002 (1.8s)

245 passed, 2 failed in 4m 23s.

Summary written to evals/summaries/2026-05-02-T14-23.yml
```

### Suite mode console output (compare mode)

```
$ skill-eval --compare

Running 247 evals against claude-sonnet-4-6 in compare mode (concurrency: 4)...

[1/247] LOAD-BEARING  EV-001  RU-001 (4.2s)
[2/247] OBSOLETE      EV-002  RU-001 (4.1s)
[3/247] INSUFFICIENT  EV-003  RU-013 (4.4s)
[4/247] HARMFUL       EV-019  RU-021 (4.0s)

Run complete in 12m 34s.

Classifications:
  Load-bearing: 198 (80%)
  Obsolete:     12  (5%)
  Insufficient: 35  (14%)
  Harmful:      2   (1%)

Notable findings:
  Obsolete (consider removal):
    RU-014  active_record_naming
    RU-019  view_helper_naming
  
  Harmful (investigate):
    RU-031  controller_naming
    RU-058  changelog_tracking

Summary written to evals/summaries/2026-05-02-T14-23.yml
```

### Targeted mode console output, single eval, single model

```
$ skill-eval --eval EV-001

Running EV-001 against RU-001 in compare mode...
Model: claude-sonnet-4-6

WITH prompt:
  contains "params.expect"     ✓
  not_contains "params.permit" ✓
  Result: PASS (2.3s)

WITHOUT prompt:
  contains "params.expect"     ✗
  not_contains "params.permit" ✓
  Result: FAIL (2.0s)

Classification: LOAD-BEARING

The prompt is doing work. The model needs the prompt loaded to produce
correct output for this eval.

Artifacts: evals/results/RU-001/EV-001/
```

### Targeted mode console output, multiple evals, single model

```
$ skill-eval --prompt-file substrate/rules/RU-001-params-expect.md

Running 4 evals against RU-001 in compare mode...
Model: claude-sonnet-4-6

EV-001  LOAD-BEARING  (4.3s)
EV-002  LOAD-BEARING  (4.1s)
EV-003  OBSOLETE      (4.4s)
EV-004  LOAD-BEARING  (4.2s)

Summary:
  Load-bearing: 3
  Obsolete:     1

Notes:
  EV-003 obsolete — model produces correct output without the prompt.

Artifacts: evals/results/RU-001/
```

### Targeted mode console output, multiple models (matrix)

```
$ skill-eval --prompt-file substrate/rules/RU-001-params-expect.md --model claude-sonnet-4-6,claude-haiku-4-5

Running 4 evals against RU-001 in compare mode across 2 models...

                       Sonnet      Haiku
RU-001 (EV-001)        LOAD        LOAD        (sonnet 4.2s, haiku 3.1s)
RU-001 (EV-002)        LOAD        LOAD        (sonnet 4.0s, haiku 2.9s)
RU-001 (EV-003)        OBSOLETE    LOAD        (sonnet 4.3s, haiku 3.2s)
RU-001 (EV-004)        LOAD        LOAD        (sonnet 4.1s, haiku 3.0s)

Per-model summary:
  claude-sonnet-4-6: 3 load-bearing, 1 obsolete
  claude-haiku-4-5:  4 load-bearing

Notes:
  EV-003 obsolete on Sonnet — consider whether the prompt is needed
  for EV-003's behavior on this model tier.

Artifacts: evals/results/RU-001/
```

Model column headers use the last meaningful path component of the model identifier, capitalized (e.g., "Sonnet" from "claude-sonnet-4-6"). If the short form is ambiguous across models in the run, fall back to the full identifier.

## Init Behavior

The `init` subcommand scaffolds a new eval entry in `evals.yml` for a substrate item. It is a pure scaffolding operation — no `claude -p` invocations, no artifacts written, no evals run.

### Invocation

```
skill-eval init --path PATH [--dry-run]
```

The `--path` flag is required. It points to a substrate file (rule, capability, pattern, procedure, or any other prompt fragment file).

### Substrate ID extraction

The substrate ID is extracted from the filename by taking the portion before the first non-ID character. Conventions assumed:

- `RU-001-params-expect.md` → `RU-001`
- `PA-002-eager-loading.md` → `PA-002`
- `CA-005-validation.md` → `CA-005`

The extraction takes the prefix matching `[A-Z]+-\d+`. Anything after the matched prefix is descriptive and discarded for the substrate ID.

If the filename does not match this pattern, error with a helpful message:

```
Error: cannot extract substrate ID from path 'foo/bar/baz.md'.
Expected filename pattern: {PREFIX}-{NUMBER}-{description}.md
Example: RU-001-params-expect.md
```

### Eval ID generation

The next available eval ID is computed by reading the existing `evals.yml`, finding the highest `EV-NNN` value across all entries, and incrementing.

If `evals.yml` does not exist, the first eval ID is `EV-001`. If it exists but is empty, also `EV-001`. If parsing fails (malformed YAML), error and refuse to add.

### Existing eval reporting

Before appending the new entry, the command reports any existing evals that test the same substrate ID:

```
Found 2 existing evals for RU-001:
  EV-022 (line 87)
  EV-023 (line 95)
```

This gives the user context for whether they're adding to an existing set or starting fresh.

If no existing evals match the substrate ID, no report is shown — just the addition.

### The placeholder entry

The appended entry has these fields:

```yaml
- id: EV-024
  tests: RU-001
  prompt_file: substrate/rules/RU-001-params-expect.md
  # TODO: describe the task the model should perform
  input: ""
  assert:
    # TODO: define assertions for what the output should contain
    - contains: ""
```

Fields filled in mechanically:

- `id` — next available EV-NNN
- `tests` — substrate ID from filename
- `prompt_file` — the path argument as provided

Fields left as placeholders with TODO comments:

- `input` — empty string, comment indicates user must supply
- `assert` — single empty `contains` assertion, comment indicates user must define

The `models:` block is not included in the placeholder. The user adds it if needed.

### Append behavior

The command appends to `evals.yml` rather than rewriting it. Existing comments, formatting choices, and ordering are preserved.

If the file does not end with a newline, one is added before the new entry. If the file is empty or doesn't exist, the file is created with the new entry as the only content (no top-level wrapper structure required — `evals.yml` is a list at the root).

### Console output

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

### Dry-run mode

`--dry-run` prints what would be added without modifying `evals.yml`:

```
$ skill-eval init --path substrate/rules/RU-001-params-expect.md --dry-run

Found 2 existing evals for RU-001:
  EV-022 (line 87)
  EV-023 (line 95)

Would add EV-024 to evals.yml (--dry-run, no changes made).

  - id: EV-024
    ...
```

### One eval per invocation

The command produces exactly one new eval per invocation. To create multiple evals for the same substrate item, the user runs the command multiple times. Each run produces a new eval with the next available ID.

This is intentional. Multiple-eval scaffolding could be added later via a `--count` flag if friction warrants, but the simpler one-per-invocation form is sufficient for the common case.

### Exit codes

- 0: eval entry successfully added (or would be added in dry-run mode)
- 2: error — invalid path, malformed evals.yml, ID extraction failure, or other startup error

The init subcommand never produces exit code 1 because it does not run evals.

## Error Handling

### Subprocess failures

`claude -p` invocation can fail for transient reasons (network blip, subprocess error, timeout). The runner retries once with a 2-second delay. If the retry also fails, the eval is marked as ERROR (distinct from assertion failure).

### Per-eval timeout

Default 60 seconds per `claude -p` invocation. Configurable in `.skill-eval.yml`. On timeout, the subprocess is killed and the eval is marked as ERROR.

### Compare mode with one run erroring

If with-prompt errors but without-prompt succeeds: report ERROR, do not classify.
If without-prompt errors but with-prompt succeeds: report ERROR, do not classify.
If both error: report ERROR, do not classify.

ERROR is distinct from the four classifications and is tracked separately in the summary.

### Malformed eval definitions

The runner validates `evals.yml` at startup. Validation failures produce specific error messages naming the offending entry and exit with code 2.

### Missing prompt files

If an eval has `prompt_file:` and the file doesn't exist, the runner errors at startup before any executions begin. This catches typos and missing files cheaply.

### Invalid CLI flag combinations

```
skill-eval --filter X --eval Y       # error: --filter and --eval are mutually exclusive
skill-eval --model X                 # error: --model is suite-mode-illegal, requires --eval or --prompt-file
skill-eval --all-models              # same as above
skill-eval --eval X --compare        # error: targeted mode is always compare
skill-eval --eval X --model A --all-models   # error: --model and --all-models are mutually exclusive
```

Errors print a helpful message and exit with code 2.

### Exit codes

- 0: all evals completed successfully (any classification, no execution errors)
- 1: at least one eval errored (subprocess failure after retry, OR in non-compare suite mode, at least one eval failed assertions)
- 2: config or startup error, including invalid flag combinations and validation failures

In compare mode, classifications are informational. A "harmful" classification does not cause exit code 1. The exit code reflects whether the tool ran successfully, not whether the substrate is healthy. CI gating uses non-compare suite mode for regression; compare mode is a survey instrument.

## Library Structure

```
pkg/skilleval/
  evaluator.go     # core: run a single eval against a prompt, return result
  compare.go       # compare mode: run twice, classify
  suite.go         # suite mode: orchestrate many evals
  targeted.go      # targeted mode: orchestrate, model resolution
  matrix.go        # matrix output rendering for multi-model
  artifacts.go     # file writing, cleanup
  classifier.go    # load-bearing / obsolete / insufficient / harmful logic
  prompt.go        # prompt construction (with-prompt and without-prompt)
  claude.go        # claude -p subprocess wrapper
  config.go        # .skill-eval.yml loading
  evals.go         # evals.yml loading and validation
  assertions.go    # assertion type implementations
  summary.go       # summary compilation
  init.go          # init subcommand: scaffold new eval entries

cmd/skill-eval/
  main.go          # CLI flag parsing, mode dispatch
  flags.go         # flag definitions
```

The core type `Evaluator` knows how to run one eval against one prompt source against one model. Compare mode wraps it. Suite mode and targeted mode orchestrate many invocations. The CLI layer dispatches to the appropriate orchestration based on flags.

A library consumer can import `pkg/skilleval` and call `Evaluator.Run` or `Evaluator.RunCompare` directly. This is forward-compatible with future tools that want to embed eval running.

## Build Phases

The tool ships value at every phase. Each phase is independently usable.

### Phase 1: Single-eval suite mode

Reads `evals.yml`, runs one eval at a time sequentially, writes per-eval artifacts, prints results to stdout. No compare mode, no concurrency, no targeted mode, no compilation step.

**Ships:** the ability to run any single eval and get a pass/fail with output captured to disk.

**Estimated size:** 250-300 lines of Go.

### Phase 2: Concurrency

Adds the worker pool and `--concurrency` flag. Same per-eval logic, parallelized.

**Ships:** dramatically faster suite runs, enabling CI integration.

**Estimated size:** +50 lines of Go.

### Phase 3: Compare mode

Adds `--compare` flag to suite mode. Each eval runs twice, classifications computed and reported.

**Ships:** the strategic survey instrument for prompt obsolescence detection.

**Estimated size:** +100 lines of Go.

### Phase 4: Summary compilation

Walks `evals/results/` after the run, builds the summary YAML, writes it to `evals/summaries/`. Standalone subcommand for compiling historical summaries.

**Ships:** full reporting, longitudinal trend data.

**Estimated size:** +80 lines of Go.

### Phase 5: Targeted mode

Adds `--prompt-file` and `--eval` flags. Targeted mode dispatches into the same execution path with different output formatting and no summary.

**Ships:** the TDD loop for prompt development.

**Estimated size:** +75 lines of Go.

### Phase 6: Multi-model targeted

Adds `--model` and `--all-models` flags. Multi-model runs produce per-model subdirectories and matrix output. Suite mode rejects `--model` with helpful error.

**Ships:** substrate strategy instrument capability for cross-model decisions.

**Estimated size:** +100 lines of Go.

### Phase 7: Init subcommand

Adds the `init` subcommand for scaffolding new eval entries. Reads a substrate file path, extracts the substrate ID, computes the next available eval ID, appends a placeholder entry to `evals.yml`.

**Ships:** ergonomic eval authoring — engineers can scaffold new evals with one command instead of hand-editing YAML.

**Estimated size:** +120 lines of Go.

### Total

Approximately 720-920 lines of Go across seven phases. Each phase is independently shippable.

## Testing Strategy

### Unit tests

The library should have unit tests for:

- Each assertion type with positive and negative cases
- Classification logic (the four states from with/without combinations)
- Prompt construction (inline and file-based, with and without input)
- Eval YAML parsing (valid forms, all validation error cases)
- Config loading (with and without `.skill-eval.yml`)
- Cleanup logic (single-model, multi-model, mixed-state)
- Model resolution (all four tiers of the resolution order)

### Integration tests

The tool should be testable against a fixture eval suite using a stub `claude -p` (a script that returns canned responses). Tests are deterministic.

## Open Implementation Questions

These are questions the implementing engineer will need to resolve, but they don't affect the design.

1. **YAML library.** `gopkg.in/yaml.v3` is standard. Use it.

2. **CLI library.** Standard library `flag` package is sufficient for this surface. No need for cobra. Subcommand handling (for `compile-summary` and `init`) requires manual `os.Args[1]` inspection but that's straightforward.

3. **Subprocess execution.** `os/exec` with context for timeout. Standard pattern.

4. **`claude -p` flag for model selection.** Verify the actual flag name when implementing. The runner must know how to pass model selection to the subprocess.

## Out of Scope

The following are intentionally not part of this tool. They may be future work.

- **No prompt rewriting or compression.** This tool tests prompts; it does not modify them. Compression is a separate tool that uses this one for validation.

- **No eval generation from substrate content.** The `init` subcommand scaffolds placeholder entries based on filename and the next available ID, but does not read substrate file contents or generate inputs/assertions automatically. Evals are written by humans; the tool only mechanizes the boilerplate.

- **No agent integration testing.** This is unit testing for prompts. Integration testing through full agents is a different harness.

- **No suite-level multi-model.** Multi-model is targeted-only. Suite-level multi-model is shell-loop territory.

- **No model variance accounting.** Each eval runs once per (model, mode) combination. If you want to account for stochasticity, run targeted mode with the same eval multiple times manually.

- **No automatic obsolescence removal.** The tool reports classifications. Decisions about removal are human judgment.

- **No CI-specific output formats.** The tool prints to stdout in human-readable format. JUnit XML, GitHub Actions annotations, etc. are the consumer's responsibility.
