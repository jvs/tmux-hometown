VERSION ?= dev

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" .

install:
	go install .

release:
	@if [ "$$(git branch --show-current)" != "main" ]; then \
		echo "error: not on main branch"; exit 1; \
	fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "error: working tree is dirty"; exit 1; \
	fi
	@if [ -z "$(VERSION)" ]; then \
		echo "error: VERSION is required (e.g. make release VERSION=v0.1.0)"; exit 1; \
	fi
	git push origin main
	git tag $(VERSION)
	git push origin $(VERSION)

.PHONY: build install release
