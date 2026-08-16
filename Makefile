.PHONY: generate test tidy fmt fmt-check vet lint check

GO ?= go
GOLANGCI_LINT ?= golangci-lint

# Library modules covered by CI. cmd/ubltr is included in test/vet/fmt locally.
LIB_MODULES := . render render/chromium
FMT_MODULES := . render render/chromium cmd/ubltr

generate:
	$(GO) generate ./schema/ubltr121
	$(GO) generate ./schema/earchive
	$(GO) generate ./codelist

test:
	$(GO) test ./...
	$(GO) test ./render/...
	$(GO) test ./render/chromium/...
	$(GO) test ./cmd/ubltr/...

tidy:
	$(GO) work sync
	$(GO) mod tidy
	cd render && $(GO) mod tidy
	cd render/chromium && $(GO) mod tidy
	cd cmd/ubltr && $(GO) mod tidy

fmt:
	@for dir in $(FMT_MODULES); do \
		echo "==> fmt $$dir"; \
		(cd $$dir && $(GOLANGCI_LINT) fmt ./...); \
	done

fmt-check:
	@fail=0; \
	for dir in $(FMT_MODULES); do \
		echo "==> fmt-check $$dir"; \
		if ! (cd $$dir && $(GOLANGCI_LINT) fmt --diff ./...); then \
			fail=1; \
		fi; \
	done; \
	exit $$fail

vet:
	$(GO) vet ./...
	$(GO) -C render vet ./...
	$(GO) -C render/chromium vet ./...
	$(GO) -C cmd/ubltr vet ./...

lint:
	@for dir in $(LIB_MODULES); do \
		echo "==> lint $$dir"; \
		(cd $$dir && $(GOLANGCI_LINT) run ./...); \
	done

check: fmt-check vet lint test
