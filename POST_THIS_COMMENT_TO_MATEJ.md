# POST THIS COMMENT ON PR #3728

**Go to:** https://github.com/knative/func/pull/3728

**Click:** "Add a comment" at the bottom

**Copy and paste this:**

---

Hi @matejvasek,

Thank you for the thorough review! I've addressed all 5 issues you identified:

## Fixes Applied

### ✅ Issue #1: TLS cert path lookup (CRITICAL)
- Now reads `TLSPath` directly from `docker context inspect` output first
- Only computes hash-based path as fallback when `TLSPath` is empty or `"<IN MEMORY>"`
- Replaced `os.Getenv("HOME")` with `os.UserHomeDir()` for cross-platform support
- Clarified in comments that certs are in `contexts/tls/<hash>/` (not in a subdirectory)

### ✅ Issue #2: contextConfig precedence logic (CRITICAL)
- Added `hostFromContext` boolean flag to track whether `dockerHost` came from context detection
- Only pass `contextConfig` to `newHttpClient()` when `hostFromContext` is true
- Makes precedence explicit: **env vars FIRST, context SECOND**

### ✅ Issue #3: Missing DOCKER_TLS_VERIFY cleanup (MEDIUM)
- Added `t.Setenv("DOCKER_TLS_VERIFY", "")` to both TLS tests
- Ensures tests are robust and don't depend on external environment

### ✅ Issue #4: os.Getenv("HOME") cross-platform (MEDIUM)
- Replaced with `os.UserHomeDir()` which works on Windows
- Added error handling to return config without TLS if home dir can't be determined

### ✅ Issue #5: CodeQL warning about InsecureSkipVerify (LOW)
- Added `#nosec G402` annotation with justification comment
- Documents that `InsecureSkipVerify` is intentionally configurable via Docker context

## Minor Nits Fixed
- Removed obvious comment about `DOCKER_CONFIG` inheritance
- Improved cert parse error handling to return nil instead of logging to stderr

## Summary

All 5 issues are now resolved. The feature will work correctly on real Docker installations with TLS contexts. All changes maintain backward compatibility and improve code clarity.

**Ready for re-review!** 🚀

**Commit:** `2f3be951` - `fix: address Matej's TLS review feedback - all 5 issues`

---

**Then click "Comment" button to post it.**

