import asyncio
import logging
from collections.abc import Callable
from contextlib import asynccontextmanager

from fastapi import APIRouter, Depends, FastAPI

from app import auth
from app.db import SessionLocal
from app.routers import admin, fertilizers, lamp, lamps, light, logs, plants, settings
from app.services import lamps as lamps_svc
from app.services.light import sync_all

SYNC_EVERY_SECONDS = 30 * 60
TICK_EVERY_SECONDS = 60
log = logging.getLogger("poliv")


def _sync_once() -> None:
    with SessionLocal() as db:
        sync_all(db)


def _tick_once() -> None:
    with SessionLocal() as db:
        lamps_svc.tick(db)


async def _every(seconds: int, job: Callable[[], None], what: str) -> None:
    while True:
        try:
            await asyncio.to_thread(job)
        except Exception:
            log.exception("фоновая задача «%s» упала", what)
        await asyncio.sleep(seconds)


@asynccontextmanager
async def lifespan(_: FastAPI):
    """Фоном: раз в полчаса свет по городу учёток; раз в минуту — расписания, досветка и розетки."""
    tasks = [
        asyncio.create_task(_every(SYNC_EVERY_SECONDS, _sync_once, "свет по городу")),
        asyncio.create_task(_every(TICK_EVERY_SECONDS, _tick_once, "лампы и розетки")),
    ]
    yield
    for task in tasks:
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
for r in (admin.router, plants.router, fertilizers.router, logs.router, lamp.router, lamps.router, lamps.yandex_router, light.router, settings.router):
    protected.include_router(r)
api.include_router(protected)


@api.get("/health", tags=["health"])
def health() -> dict[str, str]:
    return {"status": "ok"}


app.include_router(api)
