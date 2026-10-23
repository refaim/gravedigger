.PHONY: check lint test build

# PowerShell 7 provides the same checks on Windows, Linux, and macOS.
check lint test build:
	pwsh -NoProfile -File scripts/check.ps1
