from fastapi import APIRouter, Depends, FastAPI

from app import auth
from app.routers import admin, fertilizers, lamp, logs, plants, settings

app = FastAPI(
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
for r in (admin.router, plants.router, fertilizers.router, logs.router, lamp.router, settings.router):
    protected.include_router(r)
api.include_router(protected)


@api.get("/health", tags=["health"])
def health() -> dict[str, str]:
    return {"status": "ok"}


app.include_router(api)
