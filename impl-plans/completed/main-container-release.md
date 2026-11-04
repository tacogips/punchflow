# Main Merge and Container Release

**Status**: Completed
**Design Reference**: [CLI and deployment](../../design-docs/specs/command.md)
**Created**: 2026-11-04
**Last Updated**: 2026-11-04

Integrate PR #1 while preserving the Go 1.27.3 security baseline, then publish the merged container. No Go source interfaces change.

## Deliverables and Dependencies

| Task | Deliverables | Depends on | Status |
| --- | --- | --- | --- |
| Resolve conflicts | README.md, go.mod, mise.toml, Dockerfile | None | Complete |
| Repair current-toolchain lint | internal/webhook/dispatcher_test.go, internal/server/server.go, internal/engine/custom_validation_resources.go and regression tests | Resolve conflicts | Complete |
| Validate integration | Existing build, tests, lint, module verification | Resolve conflicts | Complete |
| Merge and publish | PR #1, Docker Hub amd64/arm64 image | Validation | Complete |

## Completion Criteria

- [x] Preserve application functionality and main security baseline.
- [x] Build, tests, lint and module verification pass.
- [x] PR #1 merged into main.
- [x] Container published and verified for both platforms.

## Progress Log

2026-11-04: Go 1.27.3 tests, build, vet, TypeScript, module verification and vulnerability scan pass. Linter 2.15.0 reports three unchecked test channel errors and one unreachable RawMessage case; fix these before release.

2026-11-04: PR #1 conflicts with main security update; existing Docker image is based on dfcbfde and uses the older compiler. Resolve configuration conflicts and align Docker compiler before rebuilding.

2026-11-04: Current golangci-lint v2.15.0 passes with zero issues after focused Go fixes. Independent build, full tests, vet and race verification started. Docker build uses Go 1.27.3; Docker VM DNS repaired for registry access.

2026-11-04: Independent full build, vet and uncached Go tests passed (1,651 test/subtest pass events across 27 packages). Reduced container Bun network concurrency to eight after default parallel downloads stalled; web dependency install and build then passed. Focused race checks and container smoke test pending.

2026-11-04: Independent verification completed: build/vet pass, 1,651 uncached Go test/subtest passes, 393 focused race test/subtest passes, no failures/skips/races. All 74 web tests, TypeScript checks, module verification, vulnerability scan and current lint pass.

2026-11-04: Both platform container builds pass; arm64 health/UI/authenticated API smoke passes and amd64 version runs. PR #1 merged at 78e6fe1. Published and independently verified Docker Hub tags latest, 0.1.0 and sha-78e6fe1 for linux/amd64 and linux/arm64, digest sha256:4d39bfc4d81117499b4ecb5c821d2a80be4bfff6a2520dfc7abdda5fec454c49. No open PRs remain. This completion record changes documentation only; image source is the main merge commit.
