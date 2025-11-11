# Gitea ↔ Jenkins Webhook Bridge

Сервис принимает вебхуки от Gitea по событию Pull Request и проверяет,
создалась ли соответствующая Jenkins job. Результат проверки
автоматически публикуется комментариями в PR.

## Возможности
- Асинхронная обработка входящих вебхуков с очередью и пулом воркеров
- Гибкая конфигурация регулярных выражений для разных репозиториев и паттернов job
- Настраиваемые таймауты ожидания и интервал опроса Jenkins
- Комментарии в Gitea с URL найденной job или уведомлением об отсутствии
- Поддержка HMAC-подписи вебхука Gitea
- Docker-образ и `docker-compose` для локального запуска
- Покрытие тестами с расчётом покрытия, сценарии сборки в GitHub Actions

## Быстрый старт

```bash
# Установка зависимостей и запуск тестов
make test

# Сборка бинарника
make build

# Запуск (используется config.sample.yaml)
make run
```

## Конфигурация

Все настройки описываются в YAML (см. `config.sample.yaml`):

- `server` — параметры HTTP-сервера и переменная окружения с секретом вебхука
- `processing` — размер очереди, количество воркеров, таймаут ожидания job
- `gitea` и `jenkins` — базовые URL и имена переменных окружения для токенов
- `repositories` — список поддерживаемых репозиториев и шаблонов regex

Регулярные выражения задаются через Go templates. В шаблоне доступны поля:
`PullRequest`, `PullRequestTitle`, `SourceBranch`, `TargetBranch`, `PatternName`,
`JobURL`, `Regex`, `WaitDurationText` и другие (см. `TemplateData`).

## Docker

```bash
# Сборка контейнера
make docker-build

# Запуск (ожидает .env с токенами и подменяет config при необходимости)
make docker-run

# docker-compose вариант
make docker-compose-up
```

По умолчанию внутрь контейнера копируется `config.sample.yaml`. Для
боевого использования передайте собственный конфиг:

```bash
docker run \
  -v $(pwd)/config.prod.yaml:/etc/webhook/config.yaml:ro \
  --env-file .env \
  -p 8080:8080 \
  gitea-jenkins-webhook:latest
```

## GitHub Actions

Workflow `ci.yml` (расположен в `.github/workflows/`) выполняет:
1. Сборку и линтинг
2. Тесты с покрытием
3. Сборку Docker-образа (проверочный этап)

Триггеры: Pull Request, пуши в основную ветку, теги вида `v*. *. *`.

## Настройка вебхука Gitea

1. Создайте в Gitea webhook на событие `Pull Request Events`.
2. Укажите URL: `http(s)://<host>/webhook`.
3. Задайте секрет и сохраните его в переменной окружения `WEBHOOK_SECRET`.
4. Убедитесь, что токены `GITEA_TOKEN`, `JENKINS_USER`, `JENKINS_TOKEN`
   доступны сервису (через `.env`, секреты CI/CD и т. п.).

## Тестирование и покрытие

```bash
make test      # go test ./... -coverprofile=coverage.out
make cover     # вывод сводки покрытия
```

Тесты покрывают загрузку конфигурации, обработку вебхуков, работу процессора.

## Переменные окружения

| Переменная         | Назначение                              |
|--------------------|-----------------------------------------|
| `GITEA_TOKEN`      | Токен доступа к API Gitea               |
| `JENKINS_USER`     | Логин Jenkins                           |
| `JENKINS_TOKEN`    | API Token Jenkins                       |
| `WEBHOOK_SECRET`   | Секрет для подписи вебхуков Gitea       |

Переменные можно переопределять в конфиге (через `*_env`).

## Локальная разработка

- Запустите `docker-compose up` чтобы получить сервис и, при желании,
  локальные контейнеры Gitea/Jenkins (раскомментируйте службы в compose).
- Изменения конфигурации применяются без перезапуска только при новом запуске контейнера/сервиса.

## Лицензия

MIT (см. `LICENSE`).
