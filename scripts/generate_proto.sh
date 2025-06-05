#!/bin/bash

# Создаем директорию для сгенерированных файлов
mkdir -p internal/protocol/pb

# Определяем директорию проекта и proto файлов
PROJECT_DIR=$(pwd)
PROTO_DIR="internal/protocol/proto"

# Исправляем импорты в proto файлах для корректной компиляции
echo "Fixing imports in proto files..."
find ${PROTO_DIR} -name "*.proto" -exec sed -i 's|import "internal/protocol/proto/|import "|g' {} \;

# Компилируем все .proto файлы
echo "Compiling proto files..."
protoc \
  --proto_path=${PROTO_DIR} \
  --go_out=${PROJECT_DIR}/internal/protocol \
  --go_opt=paths=source_relative \
  ${PROTO_DIR}/*.proto

# Восстанавливаем оригинальные импорты
echo "Restoring original imports..."
find ${PROTO_DIR} -name "*.proto" -exec sed -i 's|import "|import "internal/protocol/proto/|g' {} \;

echo "Protocol buffers compiled successfully" 