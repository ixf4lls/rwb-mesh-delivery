# RWB mesh delivery

Запуск всего приложения из корня репозитория:

```sh
docker compose up -d --build
```

Frontend: http://localhost:5180. Запросы браузера идут на `/api/v1` того же origin; Vite проксирует их на `http://api:8081` внутри Docker. API также доступен на http://localhost:8081, проверка БД — `/healthz`. Фиксированные IP не нужны: контейнеры обращаются друг к другу по DNS-именам сервисов. PostgreSQL доступен внутри сети как `postgres:5432`, данные сохраняются в volume `postgres_data`.

Backend реализован одним приложением: `api` обслуживает все домены, `worker` выполняет фоновые задания. Перед их стартом автоматически выполняются миграции и импорт синтетического `backend/testdata/demo`. Frontend запускается через Vite в режиме разработки.

Для запросов нужен пользовательский JWT; получить локальный токен на час:

```sh
docker compose run --rm --no-deps api token --subject demo-user --roles dispatcher,demo_admin
```

Передайте токен через предусмотренный frontend механизм авторизации. В compose заданы демонстрационные секреты; их можно переопределить переменными `MESH_USER_JWT_SECRET` и `MESH_SERVICE_JWT_SECRET` (разные строки от 32 байт).

Логи: `docker compose logs -f`. Остановка: `docker compose down` (сохраняет БД).


