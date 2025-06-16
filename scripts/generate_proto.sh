#!/bin/bash

# Создаем директорию для сгенерированных файлов
mkdir -p internal/protocol/pb

# Определяем директорию проекта и proto файлов
PROJECT_DIR=$(pwd)
PROTO_DIR="internal/protocol/proto"


# Компилируем все .proto файлы в папку internal/protocol
echo "Compiling proto files..."
protoc \
  --plugin=protoc-gen-go=/home/${USER}/go/bin/protoc-gen-go \
  --proto_path=internal/protocol/proto \
  --go_out=internal/protocol \
  --go_opt=paths=source_relative \
  internal/protocol/proto/*.proto


echo "Protocol buffers compiled successfully" 