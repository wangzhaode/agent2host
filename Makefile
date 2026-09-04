.PHONY: build install fmt fmt-check vet test race check validate package-aone release-snapshot clean

build:
	mkdir -p bin
	go build -o bin/a2h ./cmd/a2h

install:
	go install ./cmd/a2h

fmt:
	gofmt -w $$(find cmd internal -type f -name '*.go')

fmt-check:
	@files="$$(gofmt -l $$(find cmd internal -type f -name '*.go'))"; \
	if [ -n "$$files" ]; then printf 'gofmt required:\n%s\n' "$$files"; exit 1; fi

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

check: fmt-check vet test build

validate: check
	sh -n install.sh scripts/install-codex scripts/launch_agent2host_mcp scripts/package-aone-skill
	./scripts/package-aone-skill $(VERSION)
	@if command -v uv >/dev/null 2>&1 && [ -f $(HOME)/.codex/skills/.system/skill-creator/scripts/quick_validate.py ]; then uv run --with pyyaml python $(HOME)/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/agent2host; else printf '%s\n' 'Codex Skill validator not installed; skipping'; fi
	@if command -v uv >/dev/null 2>&1 && [ -f $(HOME)/.codex/skills/.system/plugin-creator/scripts/validate_plugin.py ]; then uv run --with pyyaml python $(HOME)/.codex/skills/.system/plugin-creator/scripts/validate_plugin.py .; else printf '%s\n' 'Codex plugin validator not installed; skipping'; fi
	@if command -v claude >/dev/null 2>&1; then claude plugin validate .; fi
	@if command -v qodercli >/dev/null 2>&1; then qodercli plugins validate .; fi

package-aone:
	./scripts/package-aone-skill $(VERSION)

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
