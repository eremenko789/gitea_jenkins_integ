# Gitea ↔ Jenkins Webhook Bridge

Микросервис на Go, который принимает вебхуки из Gitea о создании (или повторном открытии) Pull Request, асинхронно ожидает появления соответствующей Jenkins-джобы по заданному регулярному выражению и оставляет комментарий в PR с результатом проверки.

## Возможности
- Отсеивание репозиториев, не указанных в конфигурации.
- Настройка нескольких регулярных выражений для одного репозитория (поддерживается шаблонизация значениями PR).
- Асинхронная обработка событий с пулом воркеров и очередью.
- Периодическое опрашивание Jenkins до истечения таймаута.
- Автоматический комментарий в Gitea: ссылка на найденную джобу или уведомление об отсутствии.
- Makefile с командами сборки, тестирования, линтинга, расчёта покрытия и упаковки в Docker.
- Готовые Dockerfile и docker-compose.yml.
- Github Actions workflow для ветки `main`, Pull Request и тегов формата `v*.*.*`.

## Быстрый старт
1. Скопируйте `config.example.yaml` и заполните реальные значения.
   ```bash
   cp config.example.yaml config.yaml
   ```
2. Запустите сервис локально:
   ```bash
   go run ./cmd/webhook-service -config config.yaml
   ```
   или через docker-compose:
   ```bash
   docker compose up --build
   ```

## Конфигурация
Файл `config.yaml` описывается в YAML (пример — `config.example.yaml`):

- `server`: адрес прослушивания, секрет для подписей вебхуков, размер очереди и количество воркеров.
- `jenkins`: адрес Jenkins, credentials (basic auth), интервалы опроса и таймаут ожидания, а также `job_tree` (какие поля забирать из API).
- `gitea`: базовый URL API и токен (используется в заголовке `Authorization`).
- `repositories`: список репозиториев `org/name`. Для каждого можно указать массив `job_patterns`, а также свои интервалы и шаблоны сообщений.

Регулярные выражения и шаблоны комментариев поддерживают Go templates. Доступные поля:
`{{ .Number }}`, `{{ .Title }}`, `{{ .Repo }}`, `{{ .Sender }}`, `{{ .Timeout }}`, `{{ .JobName }}`, `{{ .JobURL }}`.

## Основные команды Makefile
- `make build` — сборка бинарника в `bin/webhook-service`.
- `make test` — тесты с `-race`.
- `make cover` — покрытие кода (`coverage.out` + сводка).
- `make lint` — `go vet`.
- `make tidy` — обновление зависимостей.
- `make docker-build` / `make docker-run` — сборка и запуск контейнера.
- `make ci` — последовательное выполнение `tidy`, `lint`, `test`, `build`, `cover`.

## GitHub Actions
Workflow `.github/workflows/ci.yml` запускается на:
- Pull Request,
- пуш в ветку `main`,
- пуш тегов `v*.*.*`.

Шаги пайплайна: checkout, установка Go 1.22, `make tidy`, `make lint`, `make test`, `make cover`, `make build`, загрузка артефакта покрытия.

## Архитектура
- `cmd/webhook-service`: точка входа, загрузка конфигурации и запуск HTTP-сервера.
- `internal/server`: HTTP-обработчики (`/webhook`, `/healthz`), проверка подписи, декодирование событий.
- `internal/processor`: очередь, worker pool, обработка PR-событий, генерация комментариев.
- `internal/jenkins`: клиент Jenkins REST API, ожидание появления джоб по regex.
- `internal/gitea`: клиент для публикации комментариев в PR.
- `pkg/webhook`: модели входящих webhook-событий.
- `config.example.yaml`: пример конфигурации.
- `Dockerfile`, `docker-compose.yml`: контейнеризация.

## Тестирование
Основные модули покрыты unit-тестами, для запуска с проверкой покрытия:
```bash
make cover
```
Файл `coverage.out` пригоден для загрузки в CI и отдельного анализа (`go tool cover -html=coverage.out`).

## Настройка Gitea и Jenkins
1. **Gitea**: создайте webhook для события Pull Request, укажите URL сервиса и HMAC secret (`server.webhook_secret`).
2. **Jenkins**: убедитесь, что имя джобы соответствует ожидаемому regex. Сервис обращается к `GET <jenkins>/api/json?tree=<job_tree>`.
3. **Gitea токен**: выдайте персональный access token с правом `write` к PR (комментарии).

## Здоровье и управление
- `GET /healthz` возвращает `200 OK` и строку `ok`.
- Завершение процесса ловит SIGINT/SIGTERM и корректно выключает сервер и worker pool.
