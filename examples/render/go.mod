module github.com/furkanmeclis/go-ubltr/examples/render

go 1.26.1

require github.com/furkanmeclis/go-ubltr/render v0.0.0

require (
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/lestrrat-go/helium v0.7.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/furkanmeclis/go-ubltr => ../..

replace github.com/furkanmeclis/go-ubltr/render => ../../render
