.PHONY: generate test tidy

generate:
	go generate ./schema/ubltr121
	go generate ./schema/earchive
	go generate ./codelist

test:
	go test ./...
	go test ./render/...
	go test ./render/chromium/...
	go test ./cmd/ubltr/...

tidy:
	go work sync
	go mod tidy
	cd render && go mod tidy
	cd render/chromium && go mod tidy
	cd cmd/ubltr && go mod tidy
