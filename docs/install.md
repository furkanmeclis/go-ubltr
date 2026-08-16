# Install

Requires **Go 1.26.1** or later.

## What to install

| Goal | Command | Extra runtime |
| --- | --- | --- |
| Invoice XML, XSD, Schematron | `go get github.com/furkanmeclis/go-ubltr` | none |
| HTML from XML + XSLT | `go get github.com/furkanmeclis/go-ubltr/render` | none (pure Go helium xslt3) |
| PDF | `go get github.com/furkanmeclis/go-ubltr/render/chromium` | Chrome or Chromium on `PATH` |

The core module’s `go.mod` does not list chromedp. A project that only imports `github.com/furkanmeclis/go-ubltr` does not download a browser.

Nested modules (`render`, `render/chromium`) are versioned independently. Until they have their own tags (`render/v0.1.0`, `render/chromium/v0.1.0`), `go get` of those paths from the proxy may fail. Use a clone of this repository and the committed `go.work` file, or a `replace` directive pointing at the local tree.

## Clone (development)

```bash
git clone https://github.com/furkanmeclis/go-ubltr.git
cd go-ubltr
go test ./...
```

`go.work` includes core, `render`, `render/chromium`, and the example modules. `make test` runs the three library modules.

## PDF runtime

`chromedp` launches the machine’s Chrome/Chromium. Docker images typically use `chromedp/headless-shell` or Debian `chromium` plus a Turkish-capable font (`fonts-noto-core` or an embedded `@font-face`).

## Generate types (maintainers)

```bash
make generate
```

Do not edit files under `schema/` or `codelist/generated.go` / `codelist/lookup.go`. Types come from the Invoice XSD; code lists come from `UBL-TR_Codelist.xml`.
