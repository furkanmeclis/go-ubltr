module github.com/furkanmeclis/go-ubltr/cmd/ubltr

go 1.26.1

replace github.com/furkanmeclis/go-ubltr => ../..

replace github.com/furkanmeclis/go-ubltr/render => ../../render

replace github.com/furkanmeclis/go-ubltr/render/chromium => ../../render/chromium

require (
	github.com/furkanmeclis/go-ubltr v0.0.0-00010101000000-000000000000
	github.com/furkanmeclis/go-ubltr/provider v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/watch v0.0.0-00010101000000-000000000000
	github.com/furkanmeclis/go-ubltr/render v0.0.0
	github.com/furkanmeclis/go-ubltr/render/chromium v0.0.0-00010101000000-000000000000
)

require (
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/chromedp v0.16.0 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/furkanmeclis/go-ubltr/provider/edm v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/izibiz v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/nes v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/nilvera v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/qnb v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/sovos v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/trendyol v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/turkcell v0.0.0
	github.com/furkanmeclis/go-ubltr/provider/uyumsoft v0.0.0
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/lestrrat-go/helium v0.7.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/furkanmeclis/go-ubltr/provider => ../../provider

replace github.com/furkanmeclis/go-ubltr/provider/watch => ../../provider/watch

replace github.com/furkanmeclis/go-ubltr/provider/edm => ../../provider/edm

replace github.com/furkanmeclis/go-ubltr/provider/izibiz => ../../provider/izibiz

replace github.com/furkanmeclis/go-ubltr/provider/nes => ../../provider/nes

replace github.com/furkanmeclis/go-ubltr/provider/nilvera => ../../provider/nilvera

replace github.com/furkanmeclis/go-ubltr/provider/turkcell => ../../provider/turkcell

replace github.com/furkanmeclis/go-ubltr/provider/uyumsoft => ../../provider/uyumsoft

replace github.com/furkanmeclis/go-ubltr/provider/qnb => ../../provider/qnb

replace github.com/furkanmeclis/go-ubltr/provider/sovos => ../../provider/sovos

replace github.com/furkanmeclis/go-ubltr/provider/trendyol => ../../provider/trendyol
