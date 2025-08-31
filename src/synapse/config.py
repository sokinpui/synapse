from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="SYNAPSE_")

    REDIS_HOST: str = "localhost"
    REDIS_PORT: int = 6666
    REDIS_DB: int = 0
    REDIS_PASSWORD: str | None = "root"


settings = Settings()
