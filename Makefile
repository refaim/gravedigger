.PHONY: lint test build

ifeq ($(OS),Windows_NT)
EXE_EXT := .exe
else
EXE_EXT :=
endif

EXE_NAME := gravedigger$(EXE_EXT)

lint:
	uv run ruff check .
	uv run mypy .

test:
	uv run pytest --cov=gravedigger --cov-report=term-missing --cov-fail-under=100

build:
	uv run nuitka --onefile --output-dir=.cache/nuitka --output-filename=$(EXE_NAME) gravedigger/cli.py
	mkdir -p bin
	cp .cache/nuitka/$(EXE_NAME) bin/
