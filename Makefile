.PHONY: generate test tidy fmt fmt-check vet lint check

GO ?= go
GOLANGCI_LINT ?= golangci-lint

# Every tracked module; LIB_MODULES skips examples/.
MODULES := $(shell scripts/modules.sh)
LIB_MODULES := $(shell scripts/modules.sh --libs)
FMT_MODULES := $(LIB_MODULES)

generate:
	$(GO) generate ./schema/ubltr121
	$(GO) generate ./schema/earchive
	$(GO) generate ./codelist

test:
	@for dir in $(MODULES); do \
		echo "==> test $$dir"; \
		$(GO) -C $$dir test ./... || exit 1; \
	done

tidy:
	$(GO) work sync
	@for dir in $(MODULES); do \
		echo "==> tidy $$dir"; \
		(cd $$dir && $(GO) mod tidy) || exit 1; \
	done

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
	@for dir in $(MODULES); do \
		echo "==> vet $$dir"; \
		$(GO) -C $$dir vet ./... || exit 1; \
	done

lint:
	@for dir in $(LIB_MODULES); do \
		echo "==> lint $$dir"; \
		(cd $$dir && $(GOLANGCI_LINT) run --config $(CURDIR)/.golangci.yml ./...) || exit 1; \
	done

check: fmt-check vet lint test
