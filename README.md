if want to use redis_om

must export REDIS_OM_URL environment variable

```bash
export REDIS_OM_URL="redis://default:root@localhost:6666/0"
```

generate proto

```
python -m grpc_tools.protoc -I=proto --python_out=src/synapse --grpc_python_out=src/synapse proto/generate.proto
```
