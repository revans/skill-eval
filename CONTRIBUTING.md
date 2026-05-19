# Contributing to skill-eval

## Prerequisites

- Go 1.22 or later
- Claude CLI installed and authenticated (see README Prerequisites section)

## Build

```bash
go build -o skill-eval ./cmd/skill-eval
```

To embed a version string in the binary:

```bash
go build -ldflags "-X main.version=1.2.3" -o skill-eval ./cmd/skill-eval
```

## Test

```bash
go test ./...
```

Tests use a stub `claude` binary injected via `$PATH` — no real API calls are made during `go test`.

## Project structure

```
cmd/skill-eval/      # main package — flag parsing and dispatch
pkg/skilleval/       # core logic — evals, runner, classifier, artifacts, init, scan
```

The `pkg/skilleval` package is the library. `cmd/skill-eval` is a thin CLI wrapper over it.

## Submitting changes

1. Fork the repo and create a branch.
2. Make your changes with tests.
3. Run `go test ./...` — all tests must pass.
4. Run `go vet ./...` — no warnings.
5. Open a pull request with a clear description of what changed and why.

## Code style

Standard Go formatting. Run `gofmt -w .` before committing. No external linter config is required — `go vet` is the bar.

Comments explain *why*, not *what*. Prefer clear naming over comments where possible.

## Reporting bugs

Open an issue. Include the output of `skill-eval --version`, your OS, and the exact command that triggered the problem.
