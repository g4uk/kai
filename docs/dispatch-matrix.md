# Dispatch Matrix

| Task type | Who | Input | Output | When NOT to use |
|---|---|---|---|---|
| "where/how does X work" | researcher | question | report ≤400 words | if the answer = 1 grep |
| new feature | test-writer → implementer → reviewer | spec+plan | PR | fix ≤10 lines |
| bugfix | test-writer (repro) → implementer | bug description | fix+test | trivial change |
| PR review | reviewer | diff | verdict | — |
| small edits, questions | NO subagents | — | — | — |
