# Настройка MariaDB для BlockVerse

## Установка MariaDB

### Ubuntu/Debian:
```bash
sudo apt update
sudo apt install mariadb-server
sudo mysql_secure_installation
```

### CentOS/RHEL:
```bash
sudo yum install mariadb-server mariadb
sudo systemctl start mariadb
sudo systemctl enable mariadb
sudo mysql_secure_installation
```

## Настройка базы данных

### Вариант 1: Автоматическая настройка (рекомендуется)
```bash
# Запустите скрипт настройки как root
sudo mysql -u root -p < scripts/setup_mariadb.sql
```

### Вариант 2: Ручная настройка
```bash
# Подключитесь к MariaDB как root
mysql -u root -p

# Выполните команды:
CREATE DATABASE IF NOT EXISTS blockverse;
CREATE USER IF NOT EXISTS 'gameuser'@'localhost' IDENTIFIED BY 'gamepass123';
GRANT ALL PRIVILEGES ON blockverse.* TO 'gameuser'@'localhost';
FLUSH PRIVILEGES;
exit;
```

## Настройка приложения

В файле `examples/rest-api-server/main.go` установите:

```go
UseMariaDB: true,
MariaConfig: auth.MariaConfig{
    Host:     "localhost",
    Port:     3306,
    Database: "blockverse",
    Username: "gameuser",
    Password: "gamepass123",
},
```

## Пользователи по умолчанию

После запуска сервера автоматически создаются:

- **admin** / ChangeMe123! (администратор)
- **test** / test123 (обычный пользователь)

## Тестирование подключения

```bash
# Проверка состояния
curl http://localhost:8088/health

# Вход администратора
curl -X POST http://localhost:8088/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"ChangeMe123!"}'

# Вход тестового пользователя
curl -X POST http://localhost:8088/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"test","password":"test123"}'
```

## Устранение проблем

### Ошибка подключения к MariaDB
1. Проверьте, что MariaDB запущена: `sudo systemctl status mariadb`
2. Проверьте логи: `sudo journalctl -u mariadb`
3. Убедитесь, что пользователь и база данных созданы

### Пользователь не найден
1. Подключитесь к базе: `mysql -u gameuser -p blockverse`
2. Проверьте пользователей: `SELECT * FROM users;`
3. При необходимости создайте заново: запустите `setup_mariadb.sql`

### Ошибка аутентификации
1. Убедитесь, что используете правильный пароль
2. Проверьте регистр символов (имена пользователей в нижнем регистре)
3. Очистите таблицу и пересоздайте: `DELETE FROM users;` и перезапустите сервер

## Безопасность

⚠️ **Важно**: В продакшене обязательно:
1. Измените пароли по умолчанию
2. Используйте сильные пароли для БД
3. Настройте SSL соединения
4. Ограничьте доступ к базе данных 