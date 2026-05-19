# Roadmap

Ideas and planned improvements for `skill-eval`. Not a commitment list — an honest record of where the tool could go next.

---

## Persistent INSUFFICIENT detection

**What**: When an eval returns `insufficient` across all models in a multi-model targeted run, surface a note pointing toward architectural investigation rather than further prompt refinement.

**Why**: A single-model INSUFFICIENT result usually means the prompt needs work. But when every model returns INSUFFICIENT regardless of what the prompt contains, the problem is likely not the prompt — the task may require an agent loop, tool access, retrieval, or state that a single-shot prompt cannot provide. The tool currently surfaces both cases identically. They warrant different responses.

**Shape**: In `--all-models` targeted mode, after classification, check whether every model returned `insufficient`. If so, append a note:

```
Note: EV-007 is insufficient across all models.
This eval may be testing a task that a prompt cannot reliably solve on its own.
Consider whether this task requires an agent, tool use, retrieval, or fine-tuning.
```

No new classification. No new flags. Pattern recognition on existing data.

---

## Summary trend analysis

**What**: A `skill-eval trend` command that reads two or more summary files and reports what changed between them — new failures, reclassified evals, shifts in classification distribution.

**Why**: Summaries accumulate as a longitudinal record but there is no built-in way to compare them. Currently you use `git diff`. A structured diff would surface regressions and improvements more clearly, especially after model upgrades where many classifications shift at once.

---

## HARMFUL exit code for CI

**What**: An optional config flag (`fail_on_harmful: true`) that makes compare mode exit 1 when any eval is classified `harmful`.

**Why**: Compare mode is a survey instrument — it never exits non-zero based on classifications, only on subprocess errors. Teams who want CI to catch prompts that actively degrade output have no mechanism for this today. An opt-in flag keeps the default behavior intact while enabling enforcement for teams that want it.
