.PHONY: build test test-race coverage clean vet fmt css

build:
	go build -o ./xoluman ./cmd/xoluman

test:
	go test ./... -count=1

test-race:
	go test ./... -count=1 -race

coverage:
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

fmt:
	gofmt -l .

# css regenerates web/static/css/tailwind.css from source. Required
# after adding or removing Tailwind classes in Go source (see
# tailwind.config.js's content-scanning paths). The compiled output is
# committed, not gitignored — go:embed needs it present at build time.
css:
	@command -v node >/dev/null 2>&1 || (echo "ERROR: Node.js not found" && exit 1)
	npx tailwindcss -c tailwind.config.js -i scripts/tailwind.input.css \
	    -o web/static/css/tailwind.css --minify
	@echo "  → web/static/css/tailwind.css"

clean:
	rm -f ./xoluman ./coverage.out ./coverage.html
