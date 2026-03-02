# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-03-02

### Added

- Interactive TUI with session browser, message viewer, and context meter
- Compaction distance estimation (turns until next automatic compaction)
- Selective message deletion with impact prediction
- Image replacement (base64 → 1x1 transparent PNG placeholder)
- Progress message removal
- ParentUuid chain repair on deletion
- Mandatory backup before any modification
- Active session protection (read-only for recently modified files)
- CLI commands: `sessions`, `stats`, `clean`, `version`
- Streaming JSONL parser with 1MB buffer for large session files
- Token estimation (text, images, tool use)
- Compaction event detection from usage metadata
