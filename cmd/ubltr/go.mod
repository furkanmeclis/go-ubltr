module github.com/furkanmeclis/go-ubltr/cmd/ubltr

go 1.26.1

replace github.com/furkanmeclis/go-ubltr => ../..

replace github.com/furkanmeclis/go-ubltr/render => ../../render

replace github.com/furkanmeclis/go-ubltr/render/chromium => ../../render/chromium

require (
	github.com/furkanmeclis/go-ubltr v0.0.0-00010101000000-000000000000
	github.com/furkanmeclis/go-ubltr/render v0.0.0
	github.com/furkanmeclis/go-ubltr/render/chromium v0.0.0-00010101000000-000000000000
)

require (
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/chromedp v0.16.0 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/lestrrat-go/helium v0.7.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)
