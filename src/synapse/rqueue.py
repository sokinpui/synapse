import json

import redis

_r = redis.Redis(
    host="localhost", port=6666, db=0, password="root", decode_responses=True
)


class RQueue:
    def __init__(self, name):
        self.name = name

    def enqueue(self, item: dict) -> None:
        _r.lpush(self.name, json.dumps(item))

    def dequeue(self) -> dict | None:
        data = _r.rpop(self.name)
        if data:
            return json.loads(data)
        return None

    def get_length(self) -> int:
        return _r.llen(self.name)
