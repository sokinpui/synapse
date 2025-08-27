#!/bin/bash

echo "Generating Python gRPC code from .proto files..."

mkdir -pv ./src/synapse/protos
mkdir -pv ./src/synapse/grpc
touch ./src/synapse/grpc/__init__.py

cd "./src/synapse" || exit

python -m grpc_tools.protoc \
  --proto_path=grpc=protos \
  --python_out=. \
  --grpc_python_out=. \
  protos/*.proto

echo "Generation complete."

gsed -i 's/from grpc import/from \. import/' ./grpc/*_grpc.py
