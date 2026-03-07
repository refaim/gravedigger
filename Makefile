.PHONY: lint test build

ifeq ($(OS),Windows_NT)
EXE_EXT := .exe
else
EXE_EXT :=
endif

EXE_NAME := gravedigger$(EXE_EXT)

lint:
	uv run ruff check .
	uv run ruff format --check .
	uv run mypy .

test:
	uv run pytest --cov=gravedigger --cov-branch --cov-report=term-missing --cov-fail-under=100

build:
	uv run nuitka --onefile --include-package-data=gravedigger --output-dir=.cache/nuitka --output-filename=$(EXE_NAME) gravedigger/cli.py
	uv run python -c "import shutil; shutil.copy('.cache/nuitka/$(EXE_NAME)', 'bin/')"
