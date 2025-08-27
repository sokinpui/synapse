import json

import redis

_r = redis.Redis(
    host="localhost", port=6666, db=0, password="root", decode_responses=True
)


class RQueue:
    def __init__(self, name):
        self.name = name

    def enqueue(self, k: str, v: dict) -> None:
        data = json.dumps({k: v})
        _r.lpush(self.name, data)

    def dequeue(self) -> dict | None:
        data = _r.rpop(self.name)
        if data:
            return json.loads(data)
        return None

    def get_length(self) -> int:
        return _r.llen(self.name)
