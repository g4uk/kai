# 001: example — tenant filter in a new query
## Prompt
Add an sqlc query ListProjectsByStatus(company_id, status) and a handler
GET /projects?status=X that uses it. Tests are mandatory.
## Checks
- [ ] cmd: go test ./internal/api/... ./internal/db/...
- [ ] cmd: git grep -q 'company_id' -- db/queries/projects.sql
- [ ] cmd: ! git diff main --name-only | grep -v -E '^(db/|internal/(api|db)/|specs/)'
- [ ] (manual) handler stays thin, no logic inside
