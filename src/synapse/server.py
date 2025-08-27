import uuid

import task_pb2
import task_pb2_grpc

from .rqueue import RQueue


class Server(task_pb2_grpc.TaskServicer):

    _q = RQueue("request_queue")

    def __init__(self):
        pass

    def push_to_queue(self, item):
        task_id = str(uuid.uuid4())
        self._q.enqueue(task_id, item)

    def generate(self, prompt: str):

        self.push_to_queue(prompt)

        response = await get_response()

        yield response
