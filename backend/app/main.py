import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import APIRouter, Depends, FastAPI

from app import auth
from app.db import SessionLocal
from app.routers import admin, fertilizers, lamp, light, logs, plants, settings
from app.services.light import sync_all

SYNC_EVERY_SECONDS = 30 * 60
log = logging.getLogger("poliv")


def _sync_once() -> None:
    with SessionLocal() as db:
        sync_all(db)


@asynccontextmanager
async def lifespan(_: FastAPI):
    """Фоном раз в полчаса: свет по городу учёток и сессии лампы по расписаниям на сегодня."""

    async def loop() -> None:
        while True:
            try:
                await asyncio.to_thread(_sync_once)
            except Exception:
                log.exception("фоновая синхронизация света упала")
            await asyncio.sleep(SYNC_EVERY_SECONDS)

    task = asyncio.create_task(loop())
    yield
    task.cancel()


app = FastAPI(
    lifespan=lifespan,
    title="Поливалка API",
    description="Трекер ухода за комнатными растениями. Авторизация: Authorize → почта учётки и пароль.",
    version="1.0.0",
    docs_url="/api/docs",
    redoc_url=None,
    openapi_url="/api/openapi.json",
)

api = APIRouter(prefix="/api")
api.include_router(auth.router)

protected = APIRouter(dependencies=[Depends(auth.current_user)])
for r in (admin.router, plants.router, fertilizers.router, logs.router, lamp.router, light.router, settings.router):
    protected.include_router(r)
api.include_router(protected)


@api.get("/health", tags=["health"])
def health() -> dict[str, str]:
    return {"status": "ok"}


app.include_router(api)
