# Failure Log

- [2026-08-04] Task 3 TDD tests failed as expected because shared notifier routing, additive Claude hook merging, Codex unset handling, and legacy-link migration were not implemented yet.
- [2026-08-04] Compatibility edge-case tests failed as expected: the modular writer did not yet migrate the exact legacy project policy alias or create missing parent directories, and TOML unset treated assignments in an unknown table as top-level.
- [2026-08-04] The modular refresh error-propagation test failed to compile as expected because `refreshTemplates` still returned only a rebuild count and could not report an invalid manifest to `runRefresh`.
- [2026-08-04] The TOML unset safety test failed as expected because removing a targeted multiline array assignment would leave its continuation lines behind and corrupt the document.

- [2026-08-04] Newly created stage documentation was hidden by the repository-wide `docs/` ignore rule; it must be force-added like the existing tracked stage records.
- [2026-08-04] Baseline `go test ./...` failed because `/tmp` is itself a Git repository, so `TestLinkedRepos` discovered `/tmp` in addition to its fixture repository; verification uses the established isolated `TMPDIR=/var/tmp/claude-sync-tests` workaround.
- [2026-08-04] The first policy-module test run failed to compile because the new module and manifest interfaces did not exist yet; this was the expected TDD red state.
- [2026-08-04] The first Task 2 test patch targeted a nonexistent `config_test.go`; the repository keeps config tests in `platform_test.go`, so the new focused test file is created explicitly.
- [2026-08-04] Task 2 path/output tests initially failed to compile because the manifest path resolver and canonical project writer had not been implemented; this was the expected TDD red state.
