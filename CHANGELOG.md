# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.1] - 2026-06-22

### Added
- Initial project structure for the `noctifab` command-line daemon interface.
- Core package scaffolding: domain entities, scheduler dag components, holding validators, and testing files.
- Added `examples` directory containing spec-driven validation targets for:
  - `url-shortener`
  - `todo-cli`
  - `weather-api`
  - `markdown-to-html`
  - `task-scheduler`
  - `frontpunch` (featuring SOLID/DI patterns, type hints, and Sidekiq parity features).
- Added `Makefile` providing targets for binary compilation, testing, and linting.
- Added GitHub Actions release workflow configured to build multi-platform binaries on tag triggers.
- Ignored `/dist/` build output folder in `.gitignore`.
