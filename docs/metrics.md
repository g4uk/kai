# Harness Metrics Log
# Fill in after EVERY significant session (/cost at session end)

| Date | Task | Approach | Tokens | $ | LOC diff | First-pass? | Human min | Note |
|------|------|----------|--------|---|----------|-------------|-----------|------|
| 2026-07-21 | walking-skeleton | /feature pipeline: subagents spec+plan (human-approved) → test-writer → implementer ×2 → reviewer ×2 → verify ×2 → retro | 104k out / 13.2M cache (Sonnet+Haiku) | $6.50 | +1285 / −1 | No | 45 | Test files left untracked — TDD pipeline needs explicit commit discipline |
| 2026-07-21 | user-auth | /feature pipeline: spec+plan (human-approved, iterative clarifying Qs) → test-writer → implementer ×2 (incl. 2 review-fix commits) → reviewer ×2 (REQUEST_CHANGES→APPROVE) → verify (READY, first pass) → retro (7 process/skill updates) → PR merge + branch cleanup | 39.7k in / 214.0k out / 31.1M cache read / 746.9k cache write (Sonnet; negligible Haiku) | $16.01 | +1883 / −21 (25 files) | Yes | 40 | SSH/gh auth friction (agent passphrase mishap, missing `workflow` OAuth scope) cost more session time than the actual review loop |
