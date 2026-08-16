# Rendering examples

Core XML/validation stays on `github.com/furkanmeclis/go-ubltr`. These programs import the nested render modules.

HTML (no Chrome):

```bash
cd examples/render/html
go run . -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml
```

Your own XSLT:

```bash
cd examples/render/custom-style
go run . -xsl invoice.xsl -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml
```

GİB `general.xslt` (the file referenced by the sample `xml-stylesheet` PI). `.xsl` and `.xslt` are the same format; only the filename differs:

```bash
cd examples/render/general
go run . -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml
```

PDF (needs Chrome or Chromium on PATH):

```bash
cd examples/render/pdf
go run . -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml
```

GİB viewer XSLT used by the official samples lives in `examples/render/general/` for local trials. It is not a core library dependency.
