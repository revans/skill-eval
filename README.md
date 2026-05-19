# skill-eval — Build Package

This directory contains the specification and phase-by-phase build prompts for the `skill-eval` Go CLI.

## What the tool is

A general-purpose prompt-impact tester. Given an eval that defines a prompt fragment and an input task, the tool runs the input both with and without the prompt and compares the results. Classifications reveal whether the prompt is doing useful work.

The most common use case is testing substrate skills (rules, capabilities, patterns, procedures) — but any prompt fragment can be tested.

## Files

- **SPEC.md** — The complete specification. Every architectural decision, flag, output format, validation rule, and behavior. Read this first.
- **PHASE-1-PROMPT.md** — Single-eval suite mode (foundation, ~250 lines of Go)
- **PHASE-2-PROMPT.md** — Concurrency (~50 lines added)
- **PHASE-3-PROMPT.md** — Compare mode and classifications (~100 lines added)
- **PHASE-4-PROMPT.md** — Summary file compilation (~80 lines added)
- **PHASE-5-PROMPT.md** — Targeted mode for prompt development (~75 lines added)
- **PHASE-6-PROMPT.md** — Multi-model targeted with matrix output (~100 lines added)
- **PHASE-7-PROMPT.md** — Init subcommand for scaffolding evals (~120 lines added)

Total: approximately 720-920 lines of Go across seven phases.

## How to use

Each phase prompt is a self-contained instruction set for Claude Code (or another coding agent) to implement that phase. Hand the prompt to the agent along with SPEC.md as context.

The phases are ordered. Each phase builds on the previous one. Do not skip ahead.

After each phase:

1. Verify the agent's output against the phase's "What you should produce" section
2. Run `go test -race ./...` to confirm tests pass
3. Try the new functionality manually to verify it works as designed
4. Commit the phase before starting the next

## Key concepts

### Prompt vs input

The eval YAML has two distinct fields:

- **`prompt`** (or `prompt_file`) — the prompt fragment being tested. This is what gets toggled on and off in compare mode.
- **`input`** — the task the model is asked to perform. Constant across both runs.

The runner constructs the with-prompt invocation as `{prompt}\n\n{input}` and the without-prompt invocation as just `{input}`.

This separation is what makes the tool general-purpose. Skills are one example of a prompt fragment. So is a few-shot example block, a persona statement, or any other text the user wants to measure the impact of.

### Two modes

- **Suite mode** is the regression catcher. CI uses it. Runs against the configured default model. Always deterministic. Eval-level model declarations are ignored in this mode.

- **Targeted mode** is the development tool. Humans use it. Supports model selection via flags or eval-level declarations. Always compare mode. Never writes summaries.

### Four classifications

Compare mode produces one of:

- **LOAD-BEARING** — passed with prompt, failed without. The prompt is doing work.
- **OBSOLETE** — passed both. The prompt is no longer needed.
- **INSUFFICIENT** — failed both. The eval, prompt, or task needs investigation.
- **HARMFUL** — failed with prompt, passed without. The prompt is making things worse.

## Why phased

Each phase ships value independently. You can stop at any phase and have a working tool.

- After Phase 1: run any single eval and see results
- After Phase 2: run the full suite quickly enough for CI
- After Phase 3: strategic survey instrument for prompt obsolescence detection
- After Phase 4: full reporting and longitudinal data
- After Phase 5: TDD loop for prompt development
- After Phase 6: substrate strategy instrument for cross-model decisions
- After Phase 7: ergonomic eval authoring via the init subcommand

Phase 1+2 is sufficient for basic regression testing. Phase 5 is the stopping point for development workflows that don't need cross-model analysis. Phase 7 is independent of Phases 2-6 — if eval scaffolding is the most pressing need, Phase 7 can ship right after Phase 1.

## Workflow recommendation

For each phase:

1. Read the phase prompt yourself before handing it off
2. Note any decisions the prompt leaves to the implementer
3. Hand the prompt + SPEC.md to your coding agent
4. Review the output against the phase's success criteria
5. Run the tests; verify functionality works
6. Iterate if needed
7. Commit when the phase is complete and tests pass

Expect more iteration on the more complex phases (3 and 6 in particular).

## After Phase 7

The tool is feature-complete. The remaining work is using it:

1. Use `skill-eval init --path ...` to scaffold evals for substrate items.
2. Fill in the input and assertions.
3. Run those evals to validate the assertion vocabulary holds up.
4. Expand to full coverage incrementally, prioritizing high-stakes prompts first.
5. Use targeted mode while editing prompts to validate changes.
6. Run suite mode in CI for regression catching.
7. Run compare mode quarterly (or after major model releases) for substrate strategy decisions.

The eval-first methodology only pays off when you have evals to run. Building the tool is the first step. Writing the evals is the work.
