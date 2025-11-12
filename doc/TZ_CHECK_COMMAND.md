# Техническое задание: Команда `check`

## 1. Общее описание

Добавить новую команду `check` в приложение `webhook-service`, которая выполняет комплексную проверку конфигурации и доступности всех компонентов системы перед запуском сервиса.

## 2. Цель

Команда `check` позволяет администратору проверить:
- Корректность файла конфигурации
- Доступность и корректность настроек сервера
- Доступность Jenkins с указанными реквизитами
- Доступность Gitea с указанными реквизитами и правами доступа
- Корректность настроек репозиториев и их соответствие реальному состоянию в Gitea и Jenkins

## 3. Требования к реализации

### 3.1. Структура команд

Приложение должно поддерживать команды (subcommands). Текущая логика запуска сервиса должна быть перенесена в команду `run`.

**Доступные команды:**
- `run` - запуск веб-сервиса (текущая функциональность)
- `check` - проверка конфигурации (новая функциональность)

**Формат вызова:**
```bash
# Запуск сервиса
./bin/webhook-service run -config <путь_к_конфигу> [-debug]

# Проверка конфигурации
./bin/webhook-service check -config <путь_к_конфигу> [-debug]
```

**Обратная совместимость:**
- Если команда не указана, приложение должно вывести справку по использованию
- Рекомендуется вывести сообщение о том, что нужно использовать команду `run` для запуска сервиса

### 3.2. Интерфейс команды `check`

**Формат вызова:**
```bash
./bin/webhook-service check -config <путь_к_конфигу>
```

**Флаги:**
- `-config` (обязательный): путь к файлу конфигурации
- `-debug` (опциональный): включить детальное логирование

**Коды возврата:**
- `0` - все проверки пройдены успешно
- `1` - обнаружены ошибки или предупреждения

### 3.3. Интерфейс команды `run`

**Формат вызова:**
```bash
./bin/webhook-service run -config <путь_к_конфигу> [-debug]
```

**Флаги:**
- `-config` (обязательный): путь к файлу конфигурации
- `-debug` (опциональный): включить детальное логирование

**Описание:**
Команда `run` запускает веб-сервис для обработки webhook'ов от Gitea. Это текущая функциональность приложения, которая должна быть обернута в команду.

### 3.4. Последовательность проверок

Команда должна выполнять проверки в следующем порядке:

#### Этап 1: Проверка файла конфигурации
1. Проверить существование файла конфигурации по указанному пути
   - Если файл не найден: вывести ошибку и завершить выполнение с кодом 1
   - Формат сообщения: `ERROR: Configuration file not found: <путь>`

#### Этап 2: Загрузка и валидация конфигурации
1. Загрузить конфигурацию используя существующую функцию `config.Load()`
2. Если загрузка не удалась: вывести ошибку и завершить с кодом 1
3. Если валидация не прошла: вывести ошибку и завершить с кодом 1
4. При успехе: вывести `✓ Configuration file loaded and validated`

#### Этап 3: Проверка настроек сервера
1. Проверить корректность настроек в секции `server`:
   - `listen_addr` - должен быть валидным адресом (не пустой)
   - `webhook_secret` - должен быть не пустым
   - `worker_pool_size` - должен быть > 0
   - `queue_size` - должен быть > 0
2. При ошибках: вывести детали и завершить с кодом 1
3. При успехе: вывести `✓ Server configuration is valid`

#### Этап 4: Проверка доступности Jenkins
1. Проверить доступность Jenkins API:
   - URL: `{jenkins.base_url}/api/json`
   - Метод: GET
   - Аутентификация: Basic Auth (username: `jenkins.username`, password: `jenkins.api_token`)
2. Проверить статус ответа:
   - Если HTTP статус 200-299: успех
   - Если HTTP статус 401/403: ошибка аутентификации
   - Если HTTP статус 404: ошибка - Jenkins не найден
   - Если таймаут/сетевая ошибка: ошибка подключения
3. При ошибках: вывести детали и завершить с кодом 1
4. При успехе: вывести `✓ Jenkins is accessible at <base_url>`

#### Этап 5: Проверка доступности Gitea
1. Проверить доступность Gitea API:
   - URL: `{gitea.base_url}/user` (эндпоинт для получения информации о текущем пользователе)
   - Метод: GET
   - Аутентификация: заголовок `Authorization: token {gitea.token}`
2. Проверить статус ответа:
   - Если HTTP статус 200-299: успех
   - Если HTTP статус 401/403: ошибка аутентификации
   - Если HTTP статус 404: ошибка - Gitea API не найден
   - Если таймаут/сетевая ошибка: ошибка подключения
3. При ошибках: вывести детали и завершить с кодом 1
4. При успехе: вывести `✓ Gitea is accessible at <base_url>`

#### Этап 6: Проверка прав комментирования в Gitea
1. Для проверки прав комментирования PR:
   - Выбрать первый репозиторий из конфигурации (если есть)
   - Если репозиториев нет: пропустить проверку с предупреждением
   - Попытаться получить информацию о репозитории через API: `GET {gitea.base_url}/repos/{owner}/{repo}`
   - Если репозиторий недоступен: вывести предупреждение (не критично)
2. Примечание: полная проверка прав комментирования требует наличия PR, поэтому эта проверка опциональна
3. При успехе: вывести `✓ Gitea repository access verified`
4. При предупреждении: вывести `⚠ Warning: Could not verify repository access (this is not critical)`

#### Этап 7: Проверка репозиториев
Для каждого репозитория из секции `repositories` выполнить следующие проверки:

##### 7.1. Проверка наличия репозитория в Gitea
1. Разобрать `name` репозитория на `owner` и `repo` (формат: `owner/repo`)
2. Выполнить запрос: `GET {gitea.base_url}/repos/{owner}/{repo}`
3. Проверить статус ответа:
   - Если HTTP статус 200-299: успех
   - Если HTTP статус 404: ошибка - репозиторий не найден
   - Если HTTP статус 403: ошибка - нет доступа к репозиторию
   - Если HTTP статус 401: ошибка аутентификации
4. При ошибках: вывести ошибку для этого репозитория и продолжить проверку остальных
5. При успехе: вывести `  ✓ Repository {name} exists in Gitea`

##### 7.2. Проверка job_root в Jenkins (если указан)
1. Если `job_root` не пустой:
   - Построить путь: `/job/{part1}/job/{part2}/.../api/json` (где части разделены `/`)
   - Выполнить запрос: `GET {jenkins.base_url}/job/{job_root}/api/json`
   - Проверить статус ответа:
     - Если HTTP статус 200-299: успех
     - Если HTTP статус 404: ошибка - job_root не найден
     - Если HTTP статус 403: ошибка - нет доступа к job_root
   - При ошибках: вывести ошибку для этого репозитория
   - При успехе: вывести `  ✓ Job root "{job_root}" exists in Jenkins`
2. Если `job_root` пустой: пропустить проверку

##### 7.3. Проверка наличия джоб в root
1. Определить путь для запроса:
   - Если `job_root` не пустой: `{jenkins.base_url}/job/{job_root}/api/json?tree=jobs[name,url,fullName]`
   - Если `job_root` пустой: `{jenkins.base_url}/api/json?tree=jobs[name,url,fullName]`
2. Выполнить запрос и получить список джоб
3. Проверить наличие джоб:
   - Если джоб нет: вывести предупреждение `  ⚠ No jobs found in root "{job_root}"` (или `"root"` если job_root пустой)
   - Если джобы есть: вывести `  ✓ Found {count} job(s) in root "{job_root}"`

##### 7.4. Проверка соответствия паттерну
1. Если джобы найдены:
   - Получить `job_pattern` из конфигурации репозитория
   - Заменить в паттерне `{{ .Number }}` на регулярное выражение для любого положительного целого числа: `\d+`
   - Скомпилировать получившееся регулярное выражение
   - Для каждой джобы проверить соответствие:
     - Проверить `job.Name` против паттерна
     - Проверить `job.FullName` против паттерна
   - Если хотя бы одна джоба соответствует: успех
   - Если ни одна джоба не соответствует: ошибка
2. При успехе: вывести `  ✓ Job pattern matches at least one job`
3. При ошибке: вывести `  ✗ No jobs match pattern "{job_pattern}"`
4. Если джоб нет: пропустить проверку паттерна

### 3.5. Формат вывода

Команда должна выводить информацию в читаемом формате:

```
Checking configuration...

✓ Configuration file loaded and validated
✓ Server configuration is valid
✓ Jenkins is accessible at https://jenkins.example.com
✓ Gitea is accessible at https://gitea.example.com/api/v1
✓ Gitea repository access verified

Checking repositories:
  Repository: org/repo-one
  ✓ Repository org/repo-one exists in Gitea
  ✓ Job root "org_name/repo_name" exists in Jenkins
  ✓ Found 5 job(s) in root "org_name/repo_name"
  ✓ Job pattern matches at least one job

  Repository: org/repo-two
  ✓ Repository org/repo-two exists in Gitea
  ⚠ No jobs found in root "root"
  ⚠ Warning: Could not verify job pattern (no jobs found)

Summary: 7 checks passed, 0 errors, 1 warning
```

**Символы статуса:**
- `✓` - успешная проверка
- `✗` - ошибка
- `⚠` - предупреждение

### 3.6. Обработка ошибок

1. **Критические ошибки** (прерывают выполнение):
   - Файл конфигурации не найден
   - Ошибка загрузки/валидации конфигурации
   - Ошибка настроек сервера
   - Jenkins недоступен
   - Gitea недоступен

2. **Ошибки репозиториев** (не прерывают выполнение):
   - Репозиторий не найден в Gitea
   - job_root не найден в Jenkins
   - Паттерн не соответствует ни одной джобе

3. **Предупреждения** (не прерывают выполнение):
   - Нет джоб в root
   - Не удалось проверить права доступа к репозиторию

### 3.7. Итоговая сводка

В конце выполнения вывести сводку:
```
Summary: {passed} checks passed, {errors} errors, {warnings} warnings
```

Если есть хотя бы одна ошибка: завершить с кодом 1
Если есть только предупреждения: завершить с кодом 0
Если все проверки успешны: завершить с кодом 0

## 4. Технические детали реализации

### 4.1. Структура кода

1. **Рефакторинг `main.go`:**
   - Переработать `main.go` для поддержки команд (subcommands)
   - Использовать `flag` пакет или стороннюю библиотеку (например, `github.com/spf13/cobra` или стандартный `flag`)
   - Текущую логику запуска сервиса вынести в функцию `runServer(configPath string, debug bool) error`
   - Создать функцию `runCheck(configPath string, debug bool) error` для команды `check`
   - Добавить обработку команд в `main()`:
     ```go
     if len(os.Args) < 2 {
         // Вывести справку
         return
     }
     command := os.Args[1]
     switch command {
     case "run":
         // Запуск сервиса
     case "check":
         // Проверка конфигурации
     default:
         // Неизвестная команда
     }
     ```

2. **Создать новый файл:** `cmd/webhook-service/check.go`
   - Реализовать функцию `runCheck(configPath string, debug bool) error`
   - Вся логика проверки конфигурации должна быть в этом файле

3. **Создать новый файл (опционально):** `cmd/webhook-service/run.go`
   - Реализовать функцию `runServer(configPath string, debug bool) error`
   - Перенести текущую логику из `main.go` в эту функцию

### 4.2. Использование существующих компонентов

- Использовать `config.Load()` для загрузки конфигурации
- Использовать `jenkins.NewClient()` для создания клиента Jenkins
- Использовать `gitea.NewClient()` для создания клиента Gitea
- Расширить клиенты методами для проверки доступности (или создать новые методы)

### 4.3. Новые методы для клиентов

**Jenkins Client:**
- `CheckAccessibility(ctx context.Context) error` - проверка доступности Jenkins
- `GetJobs(ctx context.Context, jobRoot string) ([]Job, error)` - получение списка джоб из указанного root
- `CheckJobRootExists(ctx context.Context, jobRoot string) error` - проверка существования job_root

**Gitea Client:**
- `CheckAccessibility(ctx context.Context) error` - проверка доступности Gitea
- `GetRepository(ctx context.Context, owner, repo string) error` - проверка существования репозитория
- `CheckUserPermissions(ctx context.Context) error` - проверка прав пользователя (опционально)

### 4.4. Обработка паттернов

Для проверки паттерна с номером PR:
1. Заменить в `job_pattern` все вхождения `{{ .Number }}` на `\d+`
2. Экранировать специальные символы регулярных выражений в остальной части паттерна
3. Скомпилировать как регулярное выражение
4. Проверить соответствие имен джоб

**Пример:**
- Исходный паттерн: `^PR-{{ .Number }}-build$`
- После замены: `^PR-\d+-build$`
- Проверка: `regexp.MatchString("^PR-\\d+-build$", jobName)`

### 4.5. Контекст и таймауты

Использовать контекст с таймаутом для всех HTTP-запросов:
- Таймаут по умолчанию: 10 секунд
- Контекст должен быть отменяемым для возможности прерывания

## 5. Тестирование

### 5.1. Unit-тесты

Создать тесты для:
- Проверки загрузки конфигурации
- Проверки валидации настроек сервера
- Обработки паттернов с заменой `{{ .Number }}`
- Логики проверки соответствия джоб паттерну

### 5.2. Интеграционные тесты

Создать тесты с моками для:
- Проверки доступности Jenkins (успех/ошибка)
- Проверки доступности Gitea (успех/ошибка)
- Проверки репозиториев (различные сценарии)
- Проверки job_root (существует/не существует)

## 6. Примеры использования

### 6.1. Запуск сервиса
```bash
$ ./bin/webhook-service run -config config.yaml
starting webhook service config_path=config.yaml debug=false
configuration loaded successfully server_addr=:8080 worker_pool_size=4 queue_size=100 repositories_count=2
initializing processor and server
webhook service started successfully
```

### 6.2. Запуск сервиса с отладкой
```bash
$ ./bin/webhook-service run -config config.yaml -debug
starting webhook service config_path=config.yaml debug=true
...
```

### 6.3. Справка по использованию
```bash
$ ./bin/webhook-service
Usage: webhook-service <command> [flags]

Commands:
  run     Run the webhook service
  check   Check configuration and connectivity

Use "webhook-service <command> -h" for more information about a command.
```

### 6.4. Успешная проверка конфигурации
```bash
$ ./bin/webhook-service check -config config.yaml
Checking configuration...

✓ Configuration file loaded and validated
✓ Server configuration is valid
✓ Jenkins is accessible at https://jenkins.example.com
✓ Gitea is accessible at https://gitea.example.com/api/v1
✓ Gitea repository access verified

Checking repositories:
  Repository: org/repo-one
  ✓ Repository org/repo-one exists in Gitea
  ✓ Job root "org_name/repo_name" exists in Jenkins
  ✓ Found 5 job(s) in root "org_name/repo_name"
  ✓ Job pattern matches at least one job

Summary: 7 checks passed, 0 errors, 0 warnings
```

### 6.5. Ошибка конфигурации
```bash
$ ./bin/webhook-service check -config missing.yaml
ERROR: Configuration file not found: missing.yaml
```

### 6.6. Ошибка доступности Jenkins
```bash
$ ./bin/webhook-service check -config config.yaml
Checking configuration...

✓ Configuration file loaded and validated
✓ Server configuration is valid
✗ Jenkins is not accessible at https://jenkins.example.com: connection timeout

Summary: 2 checks passed, 1 errors, 0 warnings
```

### 6.7. Ошибка в репозитории
```bash
$ ./bin/webhook-service check -config config.yaml
Checking configuration...

✓ Configuration file loaded and validated
✓ Server configuration is valid
✓ Jenkins is accessible at https://jenkins.example.com
✓ Gitea is accessible at https://gitea.example.com/api/v1

Checking repositories:
  Repository: org/repo-one
  ✓ Repository org/repo-one exists in Gitea
  ✗ Job root "wrong/path" does not exist in Jenkins
  ⚠ No jobs found in root "wrong/path"

Summary: 4 checks passed, 1 errors, 1 warnings
```

## 7. Критерии приемки

1. ✅ Приложение поддерживает команды `run` и `check`
2. ✅ Команда `run` запускает веб-сервис (текущая функциональность)
3. ✅ Команда `check` успешно запускается с флагом `-config`
4. ✅ При запуске без команды выводится справка по использованию
5. ✅ Команда `check` корректно обрабатывает отсутствие файла конфигурации
6. ✅ Команда `check` проверяет все указанные компоненты в правильном порядке
7. ✅ Команда `check` выводит понятные сообщения об ошибках и предупреждениях
8. ✅ Команда `check` возвращает правильные коды выхода (0/1)
9. ✅ Команда `check` проверяет соответствие паттернов с учетом замены `{{ .Number }}` на `\d+`
10. ✅ Команда `check` корректно обрабатывает пустой `job_root`
11. ✅ Команда `check` выводит итоговую сводку с количеством проверок
12. ✅ Обе команды поддерживают флаги `-config` и `-debug`

## 8. Дополнительные требования

1. Код должен следовать существующим стилям и паттернам проекта
2. Использовать существующий логгер (`slog`)
3. Обрабатывать все возможные ошибки
4. Документировать публичные функции
5. Добавить команду в Makefile (опционально): `make check`
6. Обновить документацию (README.md) с информацией о новых командах:
   - Запуск сервиса: `webhook-service run -config config.yaml`
   - Проверка конфигурации: `webhook-service check -config config.yaml`
7. Обновить примеры в README.md и docker-compose.yml (если необходимо)

