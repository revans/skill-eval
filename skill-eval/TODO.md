# OSS Release Checklist

Everything in this file must be done before the repo goes public.
Delete this file before the first public commit.

---

## 1. Move to its own directory and initialize git

```bash
# Copy the library to its new home (outside agentic-rails)
cp -r /path/to/agentic-rails/lib/skill-eval/skill-eval /path/to/skill-eval

cd /path/to/skill-eval
git init
git add .
git commit -m "Initial commit"
```

---

## 2. Create the GitHub repo

Go to github.com → New repository → name it `skill-eval`.

Then:

```bash
git remote add origin https://github.com/YOUR_USERNAME/skill-eval.git
git push -u origin master
```

---

## 3. Update the Go module path

**File:** `go.mod`

Change:
```
module skill-eval
```
To:
```
module github.com/YOUR_USERNAME/skill-eval
```

**File:** `cmd/skill-eval/main.go` — line 12

Change:
```go
"skill-eval/pkg/skilleval"
```
To:
```go
"github.com/YOUR_USERNAME/skill-eval/pkg/skilleval"
```

That is the only import that needs updating — `pkg/skilleval` has no imports of sibling packages.

---

## 4. Verify

```bash
go build ./...
go test ./...
```

Both must pass cleanly.

---

## 5. Update the README install command

**File:** `README.md` — Installation section

Change:
```bash
go install ./cmd/skill-eval
```
To:
```bash
go install github.com/YOUR_USERNAME/skill-eval/cmd/skill-eval@latest
```

---

## 6. Tag the first release

```bash
git tag v0.1.0
git push origin v0.1.0
```

This makes the module available via `go install ...@latest` and indexes it on pkg.go.dev.

---

## 7. Delete this file

```bash
rm TODO.md
git add TODO.md
git commit -m "Remove release checklist"
```
