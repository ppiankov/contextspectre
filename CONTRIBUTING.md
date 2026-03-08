# Contributing to ContextSpectre

Thank you for your interest in contributing.

## Getting started

```bash
git clone https://github.com/ppiankov/contextspectre.git
cd contextspectre
make deps
make build
make test
```

## Development

- Go 1.24+
- Run `make test` before submitting (uses `-race` flag)
- Run `make lint` for golangci-lint checks
- Follow conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`

## Guidelines

- Tests are mandatory for new code
- Deterministic tests only - no flaky or probabilistic tests
- Comments explain "why" not "what"
- No magic numbers - name and document constants
- Keep the scope surgical - this tool does one thing well

## Reporting issues

Open a GitHub issue with:
- What you expected
- What happened
- Steps to reproduce
- ContextSpectre version (`contextspectre version`)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
