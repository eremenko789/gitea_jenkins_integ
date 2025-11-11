SHELL := /bin/bash

BINARY := webhook-service
BUILD_DIR := bin
IMAGE := gitea-jenkins-webhook
GO_FILES := $(shell find . -name '*.go' -not -path "./vendor/*")

.PHONY: all build test lint cover fmt tidy clean docker-build docker-run docker-compose ci

all: build

fmt:
	gofmt -w $(GO_FILES)

tidy:
	go mod tidy

lint:
	go vet ./...

test:
	go test -race ./...

cover:
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/webhook-service

ci: tidy lint test build cover

clean:
	rm -rf $(BUILD_DIR) coverage.out

docker-build:
	docker build -t $(IMAGE) .

docker-run: docker-build
	docker run --rm -p 8080:8080 -v $(PWD)/config.example.yaml:/etc/webhook/config.yaml $(IMAGE)

docker-compose:
	docker compose up --build
