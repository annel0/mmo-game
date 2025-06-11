-- Настройка MariaDB для BlockVerse игры
-- Запускайте как root: mysql -u root -p < setup_mariadb.sql

-- Создаем базу данных
CREATE DATABASE IF NOT EXISTS blockverse;

-- Создаем пользователя для игры
CREATE USER IF NOT EXISTS 'gameuser'@'localhost' IDENTIFIED BY 'gamepass123';
GRANT ALL PRIVILEGES ON blockverse.* TO 'gameuser'@'localhost';
FLUSH PRIVILEGES;

-- Используем созданную базу
USE blockverse;

-- Создаем таблицу пользователей (будет создана автоматически, но можно создать заранее)
CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Создаем пользователя admin по умолчанию
-- Пароль: ChangeMe123! (будет заменен на хеш bcrypt в коде)
-- Для примера: $2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj.MFmyxNjgG
INSERT IGNORE INTO users (username, password_hash, is_admin) 
VALUES ('admin', '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj.MFmyxNjgG', TRUE);

-- Создаем обычного пользователя test
-- Пароль: test123
INSERT IGNORE INTO users (username, password_hash, is_admin) 
VALUES ('test', '$2a$12$EixZaYVK1fsbw1ZfbX3OXePaWxn96p36dH0BjQ.7m7KgKa7SZwFV2', FALSE);

-- Показываем созданных пользователей
SELECT username, is_admin, created_at FROM users; 