---
name: testing
description: >
  How to write tests in this project. Use ALWAYS when writing or
  editing test files, adding new functionality (test first!),
  or when tests fail and you need to understand the conventions.
---
# Testing

<!-- EDIT_ME: Go example; for Rails replace with RSpec conventions -->

## Rules
1. Table-driven tests by default. Case names in plain language describing behavior.
2. DB tests: real database (testcontainers / test schema), do NOT mock the data layer.
3. External APIs: interface + fake inside the package. HTTP mocks — only for testing the client itself.
4. require for preconditions, assert for checks (testify) / expect for RSpec.
5. A test for generation/calculation verifies the FULL RESULT, not the fact of a call.
6. No time.Sleep / sleep in tests — synchronize via channels/helpers.

## Template (Go)
func TestX(t *testing.T) {
    tests := []struct{ name string; in In; want Out }{
        {name: "behavior description", in: ..., want: ...},
    }
    for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { ... }) }
}

## Anti-patterns
- A test that stays green when the logic is deleted (verify with mutation testing)
- Shared mutable state between cases
