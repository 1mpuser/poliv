"""Умный дом Яндекса: список розеток и вкл/выкл. Токен — OAuth пользователя с правами
iot:view и iot:control. Документация: yandex.ru/dev/dialogs/smart-home/doc/ru/concepts/platform-protocol"""

import json
import urllib.error
import urllib.request
from dataclasses import dataclass

API = "https://api.iot.yandex.net/v1.0"
TIMEOUT = 10
ON_OFF = "devices.capabilities.on_off"


class YandexError(Exception):
    """Яндекс недоступен или устройство не выполнило команду."""


class YandexAuthError(YandexError):
    """Токен не принят (401/403): отозван, истёк или без нужных прав."""


@dataclass(frozen=True)
class Device:
    id: str
    name: str
    room: str | None
    type: str


def _call(token: str, method: str, path: str, body: dict | None = None) -> dict:
    req = urllib.request.Request(
        f"{API}{path}",
        data=None if body is None else json.dumps(body).encode(),
        method=method,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            payload = json.load(resp)
    except urllib.error.HTTPError as e:  # раньше OSError: HTTPError — его подкласс
        if e.code in (401, 403):
            raise YandexAuthError("Яндекс не принял токен") from e
        raise YandexError(f"Яндекс ответил ошибкой {e.code}") from e
    except (OSError, ValueError) as e:
        raise YandexError("Умный дом Яндекса недоступен") from e
    if payload.get("status") != "ok":
        raise YandexError(payload.get("message") or "Яндекс вернул ошибку")
    return payload


def list_devices(token: str) -> list[Device]:
    data = _call(token, "GET", "/user/info")
    rooms = {r["id"]: r["name"] for r in data.get("rooms", [])}
    return [
        Device(d["id"], d["name"], rooms.get(d.get("room")), d.get("type", ""))
        for d in data.get("devices", [])
        if any(c.get("type") == ON_OFF for c in d.get("capabilities", []))
    ]


def set_on(token: str, device_id: str, on: bool) -> None:
    data = _call(
        token,
        "POST",
        "/devices/actions",
        {"devices": [{"id": device_id, "actions": [{"type": ON_OFF, "state": {"instance": "on", "value": on}}]}]},
    )
    for device in data.get("devices", []):
        for cap in device.get("capabilities", []):
            result = cap.get("state", {}).get("action_result", {})
            if result.get("status") == "ERROR":
                raise YandexError(result.get("error_message") or result.get("error_code") or "Розетка не выполнила команду")
