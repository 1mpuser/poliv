from functools import cached_property
from zoneinfo import ZoneInfo

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    database_url: str
    app_username: str
    app_password: str
    jwt_secret: str
    jwt_expire_days: int = 30
    tz: str = "Europe/Moscow"

    @cached_property
    def zone(self) -> ZoneInfo:
        return ZoneInfo(self.tz)


settings = Settings()
