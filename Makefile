BINARY := bin/wheel
PKG    := ./cmd/wheel

.PHONY: help build run test vet fmt check tidy clean docker-up docker-down

help:
	@echo "build       compile $(BINARY)"
	@echo "run         build and run with the local .env"
	@echo "test        run the test suite"
	@echo "check       gofmt, go vet and tests, the same set CI runs"
	@echo "docker-up   start postgres and the app with compose"

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BINARY) $(PKG)

run: build
	./$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check:
	@test -z "$$(gofmt -l . )" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf bin

docker-up:
	docker compose up --build

docker-down:
	docker compose down
