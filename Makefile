BINARY := webhook-service
CMD := ./cmd/webhook-service
BIN_DIR := bin
GO := go

.PHONY: all build run test cover lint fmt clean docker-build docker-run docker-stop docker-compose-up docker-compose-down

all: build

build:
	@echo "Building..."
	$(GO) build -o $(BIN_DIR)/$(BINARY) $(CMD)

run: build
	@echo "Starting service..."
	./$(BIN_DIR)/$(BINARY) --config config.sample.yaml

test:
	$(GO) test ./... -coverprofile=coverage.out

cover: test
	$(GO) tool cover -func=coverage.out

fmt:
	$(GO) fmt ./...

lint:
	$(GO) vet ./...

clean:
	rm -rf $(BIN_DIR) coverage.out

docker-build:
	docker build -t gitea-jenkins-webhook:latest .

docker-run:
	docker run --rm -p 8080:8080 --env-file .env gitea-jenkins-webhook:latest

docker-stop:
	@docker ps --filter "name=gitea-jenkins-webhook" -q | xargs -r docker stop

docker-compose-up:
	docker-compose up --build

docker-compose-down:
	docker-compose down
