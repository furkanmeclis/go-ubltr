# Documentation

Guides for [go-ubltr](https://github.com/furkanmeclis/go-ubltr). The root [README](../README.md) is the short overview; this directory is the how-to.

| Doc | Contents |
| --- | --- |
| [Install](install.md) | Go version, `go get`, clone, CLI, what each module pulls in |
| [Usage](usage.md) | Marshal, unmarshal, Detect, XSD, Schematron, violation fields, envelopes, clone, inspect, attachments, diff |
| [Builder](builder.md) | Fluent construction for Invoice and other UBL-TR documents, money, GİB code lists |
| [e-Arşiv report](e-archive-report.md) | e-Arşiv invoice (`EARSIVFATURA`) vs e-Arşiv report (`EArsiv.xsd`) |
| [Rendering](rendering.md) | XML → HTML → PDF, stylesheets, security defaults |
| [CLI](cli.md) | `ubltr validate`, `inspect` (text or JSON), `render`, `generate` |
| [Testing](testing.md) | `make test`, `make check`, core-only runs, PDF skip, fixtures |

Community files at the repository root: [CONTRIBUTING.md](../CONTRIBUTING.md), [SECURITY.md](../SECURITY.md), [CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md), [ROADMAP.md](../ROADMAP.md), [CHANGELOG.md](../CHANGELOG.md).

Runnable programs live in [`examples/render/`](../examples/render/).
