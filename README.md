# Gitea-Jenkins Integration Service

Микросервис для интеграции Gitea и Jenkins. Сервис отслеживает создание Pull Request в Gitea и проверяет создание соответствующих джоб в Jenkins.

## Описание

Сервис получает вебхуки от Gitea при создании Pull Request и асинхронно обрабатывает их:
- Проверяет, создалась ли джоба в Jenkins для соответствующего PR по регулярному выражению
- Для разных репозиториев можно настраивать разные регулярные выражения в конфигурации
- Репозитории, не указанные в конфиге, игнорируются
- Ожидает создания джобы в течение настраиваемого времени
- Если джоба создалась - оставляет комментарий с ссылкой в PR
- Если джоба не создалась - оставляет комментарий об этом в PR

## Требования

- Go 1.21 или выше
- Docker и Docker Compose (для запуска в контейнерах)
- Gitea с настроенным вебхуком
- Jenkins с доступом к API

## Установка и запуск

### Локальная разработка

1. Клонируйте репозиторий:
```bash
git clone <repository-url>
cd gitea-jenkins-integ
```

2. Установите зависимости:
```bash
make deps
# или
go mod download
```

3. Скопируйте пример конфигурации:
```bash
cp config.yaml.example config.yaml
```

4. Отредактируйте `config.yaml` с вашими настройками:
```yaml
server:
  port: 8080
  host: "0.0.0.0"

gitea:
  base_url: "http://your-gitea:3000"
  token: "your-gitea-token"

jenkins:
  base_url: "http://your-jenkins:8080"
  username: "your-username"
  token: "your-jenkins-token"

webhook:
  timeout_seconds: 300

repositories:
  - owner: "your-org"
    name: "your-repo"
    job_pattern: "^your-repo-pr-{pr_number}$"
    timeout_seconds: 300
```

5. Запустите сервис:
```bash
make run
# или
go run ./cmd/server
```

### Docker Compose

1. Отредактируйте `config.yaml` с вашими настройками

2. Запустите все сервисы:
```bash
make docker-run
# или
docker-compose up -d
```

3. Остановите сервисы:
```bash
make docker-stop
# или
docker-compose down
```

### Docker

1. Соберите образ:
```bash
make docker-build
# или
docker build -t gitea-jenkins-integ .
```

2. Запустите контейнер:
```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  gitea-jenkins-integ
```

## Конфигурация

### Структура конфигурации

- `server` - настройки HTTP сервера
  - `port` - порт для прослушивания
  - `host` - хост для прослушивания
  
- `gitea` - настройки Gitea
  - `base_url` - базовый URL Gitea
  - `token` - токен доступа к Gitea API
  
- `jenkins` - настройки Jenkins
  - `base_url` - базовый URL Jenkins
  - `username` - имя пользователя Jenkins
  - `token` - токен доступа к Jenkins API
  
- `webhook` - настройки вебхуков
  - `timeout_seconds` - таймаут ожидания создания джобы (по умолчанию)
  
- `repositories` - список репозиториев для отслеживания
  - `owner` - владелец репозитория
  - `name` - имя репозитория
  - `job_pattern` - регулярное выражение для поиска джобы в Jenkins
    - Можно использовать плейсхолдер `{pr_number}` для подстановки номера PR
  - `timeout_seconds` - таймаут для конкретного репозитория (опционально)

### Примеры паттернов

- `^my-repo-pr-{pr_number}$` - точное совпадение с номером PR
- `^build-.*-pr-{pr_number}$` - джобы начинающиеся с "build-" и содержащие номер PR
- `^.*-pr-{pr_number}-.*$` - джобы содержащие номер PR в середине

## Настройка вебхука в Gitea

1. Перейдите в настройки репозитория → Webhooks
2. Добавьте новый вебхук:
   - URL: `http://your-service:8080/webhook`
   - Content type: `application/json`
   - Events: выберите "Pull Request"
3. Сохраните вебхук

## API

### POST /webhook

Принимает вебхук от Gitea при создании Pull Request.

**Пример запроса:**
```json
{
  "action": "opened",
  "number": 123,
  "pull_request": {
    "id": 456,
    "number": 123,
    "title": "Test PR",
    "state": "open",
    "head": {
      "ref": "feature-branch",
      "sha": "abc123"
    },
    "base": {
      "ref": "main",
      "sha": "def456"
    },
    "html_url": "http://gitea/repo/pulls/123"
  },
  "repository": {
    "id": 789,
    "name": "test-repo",
    "full_name": "test-org/test-repo",
    "owner": {
      "login": "test-org"
    }
  }
}
```

### GET /health

Health check эндпоинт для проверки работоспособности сервиса.

**Ответ:**
```json
{
  "status": "ok"
}
```

## Разработка

### Команды Makefile

- `make build` - собрать приложение
- `make run` - собрать и запустить приложение
- `make test` - запустить тесты
- `make coverage` - запустить тесты с отчетом о покрытии
- `make lint` - запустить линтеры
- `make fmt` - отформатировать код
- `make vet` - запустить go vet
- `make clean` - очистить артефакты сборки
- `make docker-build` - собрать Docker образ
- `make docker-run` - запустить через docker-compose
- `make docker-stop` - остановить docker-compose
- `make check` - запустить все проверки (fmt, vet, lint, test)
- `make deps` - обновить зависимости
- `make help` - показать справку

### Запуск тестов

```bash
# Все тесты
make test

# С покрытием кода
make coverage

# Откроет coverage.html в браузере
open coverage.html
```

### Линтинг

Установите golangci-lint:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Запустите линтинг:
```bash
make lint
```

## CI/CD

Проект использует GitHub Actions для автоматизации:
- Тестирование при каждом PR и push в main/master
- Линтинг кода
- Сборка Docker образа при push в main/master и при создании тегов v*.%.%

## Структура проекта

```
.
├── cmd/
│   └── server/
│       └── main.go          # Точка входа приложения
├── internal/
│   ├── config/              # Конфигурация
│   ├── gitea/               # Клиент Gitea API
│   ├── jenkins/             # Клиент Jenkins API
│   ├── models/              # Модели данных
│   └── webhook/             # Обработчик вебхуков
├── .github/
│   └── workflows/
│       └── ci.yml           # GitHub Actions workflow
├── config.yaml.example      # Пример конфигурации
├── Dockerfile               # Docker образ
├── docker-compose.yml       # Docker Compose конфигурация
├── Makefile                 # Команды сборки и тестирования
└── README.md                # Документация
```

## Лицензия

См. файл LICENSE
