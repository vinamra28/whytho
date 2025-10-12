.PHONY: build run test clean docker-build docker-run mod-tidy

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=whytho
BINARY_UNIX=$(BINARY_NAME)_unix

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v cmd/main.go

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

run:
	$(GOBUILD) -o $(BINARY_NAME) -v cmd/main.go
	./$(BINARY_NAME)

mod-tidy:
	$(GOCMD) mod tidy

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v cmd/main.go

# Docker
docker-build:
	docker build -t $(BINARY_NAME) .

docker-run:
	docker run -p 8080:8080 --env-file .env $(BINARY_NAME)

docker-compose-up:
	docker-compose up --build

docker-compose-down:
	docker-compose down

# Development
dev:
	air -c .air.toml

install-air:
	go install github.com/cosmtrek/air@latest