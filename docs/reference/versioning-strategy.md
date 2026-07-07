# Versioning Strategy

## Status

Accepted (2026-07-07)

## Canonical Version Source

The canonical version is the `VERSION` file at repository root. The versioning policy is defined in this document.

The `VERSION` file is read at build time and injected via ldflags:

```bash
go build -ldflags="-X main.Version=$(cat VERSION)" -o kusanagi.exe ./cmd/kusanagi/
```

This is what the binary reports at startup. All other references (CHANGELOG, git tags, release artifacts) must match `VERSION`.

## Versioning Scheme

```
v<major>.<minor>.<patch>
```

| Bump | When | Examples |
|------|------|----------|
| `+0.0.1` (patch) | Bug fixes, tool tweaks, doc updates, minor refactors | Fixing VAD sensitivity, adjusting audio buffer size, updating MCP server dependency |
| `+0.1.0` (minor) | New capabilities, architecture changes, dependency adds | Adding conversation history, adding new agent skills, switching audio backend |
| `+1.0.0` (major) | Stable release with proven architecture | Production-ready with field testing |

**Current trajectory:** v0.1.x (initial release, core loop) → v0.2.x (memory, persistence, advanced agent features) → v1.0.0 (stable release)

## Git Tagging

Every release must have a lightweight tag matching the version:

```
v0.1.0  ← tagged on the release commit
v0.1.1  ← tagged on the release commit
```

Tags are immutable once pushed. If a release is faulty, bump the patch and re-tag. Never delete and recreate a pushed tag.

## Changelog Convention

`docs/meta/CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com) with sections:

- `### Added` — new tools, new capabilities
- `### Changed` — modifications to existing tools or behavior
- `### Fixed` — bug fixes
- `### Removed` — removed tools or features
- `### Security` — security-related changes

A changelog entry is required for every release.

## Release Process

```
[1] Code complete — all changes for the release are merged
[2] Bump version in VERSION file
[3] Update docs/meta/CHANGELOG.md with the new version heading
[4] Run pre-release gates:
      - go vet ./...
      - go build ./cmd/kusanagi/
[5] Commit: "release: vX.Y.Z"
[6] Tag:   git tag vX.Y.Z
[7] Push:  git push && git push origin vX.Y.Z
[8] CI/CD auto-builds and creates GitHub Release
```

## Commit Strategy

Use squash-merges into `master` — each release is a single commit on the default branch. This keeps the release history clean.

## Pre-Release Gates

| Gate | Command | Fail action |
|------|---------|-------------|
| Lint | `go vet ./...` | Fix warnings |
| Build | `go build ./cmd/kusanagi/` | Fix compilation |

## Cross-References

- `../meta/CHANGELOG.md` — release history
- `VERSION` — canonical version source
- `../../.github/workflows/release.yml` — CI/CD workflow
