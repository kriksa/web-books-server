.PHONY: web embed liberama build test run clean

ROOT := $(CURDIR)
WEB := $(ROOT)/web
EMBED := $(ROOT)/internal/spaembed/spa
ASSETS := $(ROOT)/assets
LIBERAMA ?= $(ROOT)/third_party/liberama

web:
	cd $(WEB) && npm ci
	@$(MAKE) web-assets
	cd $(WEB) && npm run build

web-assets:
	rm -rf $(WEB)/public
	mkdir -p $(WEB)/public/backgrounds
	cp -f $(ASSETS)/favicon.svg $(WEB)/public/ 2>/dev/null || true
	cp -f $(ASSETS)/favicon.ico $(WEB)/public/ 2>/dev/null || true
	cp -f $(ASSETS)/apple-touch-icon.png $(WEB)/public/ 2>/dev/null || true
	cp -Rf $(ASSETS)/backgrounds/. $(WEB)/public/backgrounds/ 2>/dev/null || true

embed: web
	test -f $(EMBED)/index.html
	test -f $(EMBED)/reader.html

liberama:
	@test -d $(LIBERAMA) || (echo "Missing $(LIBERAMA) — clone Liberama to third_party/liberama" && exit 1)
	./scripts/build-liberama.sh

build: embed
	go mod tidy
	go build -ldflags="-s -w" -o bin/web_books ./cmd/web_books

build-full: embed liberama
	go mod tidy
	go build -ldflags="-s -w" -o bin/web_books ./cmd/web_books

test:
	go test ./...

run: build
	./bin/web_books

clean:
	rm -rf $(WEB)/node_modules $(WEB)/dist $(EMBED)/*
	rm -rf bin/web_books bin/web_books.exe
	@touch $(EMBED)/.gitkeep
