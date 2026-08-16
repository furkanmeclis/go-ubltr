# Security policy

## Supported versions

This project is pre-release. Please report vulnerabilities against the latest `main` branch.

## Reporting a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/furkanmeclis/go-ubltr/security/advisories/new).

Do **not** open a public issue for security problems.

Please include:

- Affected module path (`github.com/furkanmeclis/go-ubltr`, `…/render`, or `…/render/chromium`)
- A minimal, **synthetic** reproduction
- Impact (for example XXE, unexpected network access during XSLT or PDF print)

We will acknowledge the report and say whether a fix is planned.

## What not to send

Do not include real e-Fatura / e-Arşiv XML, taxpayer identifiers (TCKN / VKN), credentials, customer data, or production attachments. Redact or synthesize all personal and tax data.

## Scope notes

- Core XML parsing uses `encoding/xml`. XSD and Schematron run through helium.
- The HTML renderer ignores host `document()` / `xsl:include` and `xsi:schemaLocation` on input XML.
- PDF printing blocks `http(s)://` and `file://` loads; prefer `data:` URLs for images.
- This library does not implement XML signature, XAdES, or GİB web-service authentication.
