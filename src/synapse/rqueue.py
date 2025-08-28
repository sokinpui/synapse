import json

import redis.asyncio as redis


class RQueue:
    def __init__(self, redis_client: redis.Redis, name: str):
        self._redis = redis_client
        self.name = name

    async def enqueue(self, item: dict) -> None:
        await self._redis.lpush(self.name, json.dumps(item))

    async def get_length(self) -> int:
        return await self._redis.llen(self.name)

    async def dequeue(self, timeout: int = 0) -> dict | None:
        data = await self._redis.brpop(self.name, timeout=timeout)
        if data:
            return json.loads(data[1])
        return None
