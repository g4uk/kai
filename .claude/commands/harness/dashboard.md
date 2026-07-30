---
description: Build the performance dashboard from docs/metrics.md and open it
---
Build the dashboard:

```
./build/dashboard.sh $ARGUMENTS
```

No `$ARGUMENTS` — builds from the current project's own `docs/metrics.md`. Pass a
path to build from a different project's metrics instead (e.g. when running this
from the harness-kit repo itself against an installed project).

Then open the result: `dist/dashboard.html` — `open` on macOS, `xdg-open` on Linux,
otherwise just print the path.

If the build says `docs/metrics.md not found yet` or `no parseable rows` — say
that plainly, don't treat it as done. It means no `/harness:log-metrics` entries
exist yet.
