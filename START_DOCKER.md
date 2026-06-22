# Запуск VideoHub через Docker

## Первый запуск

1. Запустите Docker Desktop и дождитесь статуса **Engine running**.
2. Откройте PowerShell в папке проекта:

   ```powershell
   cd C:\Users\user\Desktop\DIplom
   ```

3. Соберите и запустите весь проект:

   ```powershell
   docker compose up -d --build
   ```

4. Проверьте контейнеры:

   ```powershell
   docker compose ps
   ```

5. Откройте приложение: http://localhost:8081

Демо-вход администратора: `admin` / `admin1`.

Дополнительные адреса:

- API: http://localhost:8080
- pgAdmin: http://localhost:5050 (`admin@videohub.com` / `admin`)
- PostgreSQL с компьютера: `localhost:5433`

## Следующие запуски

Если проект уже был собран:

```powershell
cd C:\Users\user\Desktop\DIplom
docker compose up -d
```

После изменения исходников пересоберите контейнеры:

```powershell
docker compose up -d --build
```

## Остановка

Остановить проект, сохранив базу данных и загруженные файлы:

```powershell
docker compose down
```

Посмотреть логи:

```powershell
docker compose logs -f
```

Не используйте `docker compose down -v` без необходимости: команда удалит Docker-тома с базой PostgreSQL и настройками pgAdmin.

## Если Docker Engine не запускается

Перезагрузите Windows, запустите Docker Desktop и дождитесь **Engine running**. Затем повторите:

```powershell
cd C:\Users\user\Desktop\DIplom
docker compose up -d --build
```
