# План реализации юнит-тестов

## Обзор

Проект не имеет ни одного теста. Добавим юнит-тесты для всех пакетов, сфокусировавшись на чистых функциях и ключевой бизнес-логике. Используем стандартную библиотеку `testing` и `net/http/httptest` для мокирования HTTP-серверов.

---

## Файл 1: `app/checks/check_test.go`

### 1.1 `TestFormatTimeAgo` — чистая функция форматирования времени
- Кейсы: zero time → "never", секунды, минуты, часы, дни назад

### 1.2 `TestShouldSendSSLNotification` — логика дедупликации SSL-уведомлений
- Кейсы: zero time (первый раз) → true, <24ч назад → false, >24ч назад → true

### 1.3 `TestCheckServerStatus` — проверка здоровья сервера через httptest
- Успешный ответ (200) → IsOk=true, ResponseTime > 0
- Ответ 500 → IsOk=false, ErrorMessage содержит status code
- Ответ 301 → IsOk=false
- Проверка контента: ожидаемый контент найден → IsOk=true, ContentMatched=true
- Проверка контента: ожидаемый контент НЕ найден → IsOk=false
- Таймаут сервера → IsOk=false
- Невалидный URL → IsOk=false

### 1.4 `TestConfigureHttpClient` — установка таймаутов
- Проверяем что timeout и TLS timeout применяются

### 1.5 `TestSetGlobalSSLExpiryThreshold` — установка глобального порога SSL
- Проверяем что значение меняется

---

## Файл 2: `app/checks/storage_test.go`

### 2.1 `TestInitStorage` — создание файла хранилища
- Создаёт директорию и файл, если не существует
- При повторном вызове не перезаписывает существующий файл

### 2.2 `TestSaveAndReadChecksData` — цикл записи/чтения
- Сохраняем данные → читаем → данные совпадают
- Сохраняем пустые HealthChecks → читаем → пустая map

### 2.3 `TestReadChecksDataConcurrent` — потокобезопасность
- Параллельные Read/Save не вызывают race condition (запуск с `-race`)

> Примечание: для тестов storage нужно подменить `storageLocation` на временную директорию. Для этого экспортируем функцию `SetStorageLocation()` или используем переменную пакета.

---

## Файл 3: `app/events/superuser_test.go`

### 3.1 `TestIsSuper` — авторизация пользователей
- Точное совпадение → true
- Регистронезависимое совпадение → true
- С префиксом "/" → true
- Неизвестный пользователь → false
- Пустой список superuser'ов → false
- Пустой username → false

---

## Файл 4: `app/events/telegram_test.go`

### 4.1 `TestGetFullServerUrl` — нормализация URL
- URL без протокола → добавляет "https://"
- URL с "http://" → не меняет
- URL с "https://" → не меняет
- Пустая строка → пустая строка

### 4.2 `TestGetServer` — парсинг аргументов команды
- `/add example.com myserver` → Url="https://example.com", Name="myserver"
- `/add example.com` → Url="https://example.com", Name="example.com"
- `/add` без аргументов → пустые поля

> Примечание: `getServer` и `getFullServerUrl` — приватные функции. Нужно либо экспортировать их (переименовать в `GetServer`/`GetFullServerUrl`), либо тестировать из того же пакета `events`. Выбираем второй вариант — тесты в том же пакете.

---

## Необходимые изменения в продакшн-коде

1. **`app/checks/storage.go`**: Добавить `SetStorageLocation(path string)` для тестирования с временными директориями
2. **`app/checks/check.go`**: Экспортировать `checkServerStatus` → `CheckServerStatus` для тестирования из пакета (или тестировать из того же пакета). Выбираем тестирование из того же пакета (файл `check_test.go` в пакете `checks`)

---

## Порядок реализации

1. `app/events/superuser_test.go` — самый простой, без зависимостей
2. `app/events/telegram_test.go` — тесты URL-парсинга
3. `app/checks/storage_test.go` — добавить `SetStorageLocation()`, написать тесты storage
4. `app/checks/check_test.go` — тесты FormatTimeAgo, shouldSendSSLNotification, checkServerStatus с httptest

---

## Запуск

```bash
go test ./... -v -race
```
