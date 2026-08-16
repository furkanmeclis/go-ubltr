.PHONY: generate test tidy

generate:
	go generate ./schema/ubltr121

test:
	go test ./...

tidy:
	go mod tidy
