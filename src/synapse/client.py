import requests


class Client:
    def __init__(self, host: str = "localhost", port: int = 6666):
        self._host = host
        self._port = port
        self._url = f"http://{self._host}:{self._port}"

    def request(self, payload: dict) -> dict:

