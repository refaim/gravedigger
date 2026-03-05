# Gravedigger

Resource unpacker/repacker for Dangerous Dave in the Haunted Mansion.

## Development

- Python 3.13+, dependencies managed with uv
- Run linters: `make lint` (ruff + mypy)
- Run tests: `make test` (pytest with coverage, fails under 100%)
- Build binary: `make build` (nuitka, output in bin/)
