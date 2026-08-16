## Summary

<!-- What does this change, and why? -->

## Checklist

- [ ] `make check` passes locally (or I explain why a check cannot run).
- [ ] Tests cover the behaviour change.
- [ ] User-visible changes are listed in `CHANGELOG.md` under `[Unreleased]`.
- [ ] This pull request does **not** include real invoice XML, taxpayer identifiers (TCKN / VKN), credentials, or customer data.
- [ ] Generated files under `schema/` / `codelist/` were produced with `make generate`, not edited by hand.
- [ ] The core module still has no chromedp (`GOWORK=off go list -m all | grep chromedp` is empty) unless this PR only touches nested modules.
