# Documentation

Guides for [go-ubltr](https://github.com/furkanmeclis/go-ubltr). The root [README](../README.md) is the short overview; this directory is the how-to.

| Doc | Contents |
| --- | --- |
| [Install](install.md) | Go version, `go get`, clone, what each module pulls in |
| [Usage](usage.md) | Marshal, unmarshal, Detect, XSD, Schematron, clone, inspect, attachments, diff |
| [Builder](builder.md) | Fluent construction for Invoice and other UBL-TR documents, money, GİB code lists |
| [e-Arşiv report](e-archive-report.md) | e-Arşiv invoice (`EARSIVFATURA`) vs e-Arşiv report (`EArsiv.xsd`) |
| [Rendering](rendering.md) | XML → HTML → PDF, stylesheets, security defaults |
| [Testing](testing.md) | `make test`, core-only runs, PDF skip, fixtures |

Runnable programs live in [`examples/render/`](../examples/render/).
