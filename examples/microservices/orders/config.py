"""Runtime settings — read once at import time."""

import os


class Settings:
    APP_PORT: int = int(os.getenv("APP_PORT", "5000"))
    DATABASE_URL: str = os.getenv("DATABASE_URL", "")
    REDIS_URL: str = os.getenv("REDIS_URL", "")
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "info")


settings = Settings()
