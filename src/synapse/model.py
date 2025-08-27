from pydantic import BaseModel


class Request(BaseModel):
    prompt: str
    temperature: float | None
    top_p: float | None
    top_k: int | None
    output_lenght: int | None
    # image: str | None
    # videos: str | None


class Response(BaseModel):
    text: str
    # image: str | None
    # videos: str | None
