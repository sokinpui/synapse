from abc import ABC, abstractmethod
from typing import Any


class BaseWorker(ABC):

    @abstractmethod
    async def run(self) -> None:
        pass

    @abstractmethod
    async def dequeue(self) -> None:
        pass

    @abstractmethod
    async def process(self, task: dict) -> Any:
        pass
