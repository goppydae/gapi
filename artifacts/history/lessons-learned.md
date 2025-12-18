# Lessons Learned: GAPI

## Architecture & Design

- **Lesson**: CVRL establishes a clear boundary between agentic metadata and user-facing documentation.
  - Evidence: [goppydae-silo walkthrough](goppydae-silo/artifacts/history/runs/run-20251218-ci-migration/walkthrough.md)

- **Lesson**: Linter diagnostics should be validated against actual project state during migration; "file is empty" errors in NDJSON logs are often false positives during initial repository bootstrapping.
  - Evidence: [post_history_fix_verify.log](artifacts/logs/post_history_fix_verify.log)
