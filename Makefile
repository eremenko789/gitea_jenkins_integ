.PHONY: build test lint run clean docker-build docker-run coverage help

# Переменные
BINARY_NAME=gitea-jenkins-integ
DOCKER_IMAGE=gitea-jenkins-integ
VERSION?=latest

# Сборка приложения
build:
	@echo "Building $(BINARY_NAME)..."
	@go build -o bin/$(BINARY_NAME) ./cmd/server
	@echo "Build complete: bin/$(BINARY_NAME)"

# Запуск приложения
run: build
	@echo "Running $(BINARY_NAME)..."
	@./bin/$(BINARY_NAME)

# Тестирование
test:
	@echo "Running tests..."
	@go test -v ./...

# Тестирование с покрытием
coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out
	@echo "Coverage report generated: coverage.html"

# Линтинг
lint:
	@echo "Running linters..."
	@golangci-lint run ./... || (echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)

# Форматирование кода
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Веттинг
vet:
	@echo "Running go vet..."
	@go vet ./...

# Очистка
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

# Сборка Docker образа
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE):$(VERSION) .
	@echo "Docker image built: $(DOCKER_IMAGE):$(VERSION)"

# Запуск через docker-compose
docker-run:
	@echo "Starting services with docker-compose..."
	@docker-compose up -d
	@echo "Services started"

# Остановка docker-compose
docker-stop:
	@echo "Stopping services..."
	@docker-compose down
	@echo "Services stopped"

# Полная проверка (тесты + линтинг + форматирование)
check: fmt vet lint test
	@echo "All checks passed!"

# Установка зависимостей
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies updated"

# Помощь
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  run          - Build and run the application"
	@echo "  test         - Run tests"
	@echo "  coverage     - Run tests with coverage report"
	@echo "  lint         - Run linters"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  clean        - Clean build artifacts"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Start services with docker-compose"
	@echo "  docker-stop  - Stop docker-compose services"
	@echo "  check        - Run all checks (fmt, vet, lint, test)"
	@echo "  deps         - Download and update dependencies"
	@echo "  help         - Show this help message"
