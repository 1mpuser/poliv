import io
import json
import urllib.error

import pytest

from app.services import yandex


class FakeUrlopen:
    def __init__(self, payload=None, error=None):
        self.payload, self.error, self.requests = payload, error, []

    def __call__(self, req, timeout):
        self.requests.append(req)
        if self.error is not None:
            raise self.error
        return io.BytesIO(json.dumps(self.payload).encode())


def http_error(code):
    return urllib.error.HTTPError("https://api.iot.yandex.net", code, "err", {}, None)


USER_INFO = {
    "status": "ok",
    "rooms": [{"id": "r1", "name": "Спальня"}],
    "devices": [
        {"id": "d1", "name": "Лампа цитрусы", "room": "r1", "type": "devices.types.socket",
         "capabilities": [{"type": "devices.capabilities.on_off"}]},
        {"id": "d2", "name": "Датчик", "room": None, "type": "devices.types.sensor", "capabilities": []},
    ],
}


def test_list_devices_keeps_switchable_with_room(monkeypatch):
    fake = FakeUrlopen(USER_INFO)
    monkeypatch.setattr(yandex.urllib.request, "urlopen", fake)
    assert yandex.list_devices("tok") == [yandex.Device("d1", "Лампа цитрусы", "Спальня", "devices.types.socket")]
    assert fake.requests[0].full_url == "https://api.iot.yandex.net/v1.0/user/info"
    assert fake.requests[0].get_header("Authorization") == "Bearer tok"


def test_set_on_sends_on_off_action(monkeypatch):
    fake = FakeUrlopen({"status": "ok", "devices": [{"id": "d1", "capabilities": [
        {"type": "devices.capabilities.on_off", "state": {"instance": "on", "action_result": {"status": "DONE"}}}]}]})
    monkeypatch.setattr(yandex.urllib.request, "urlopen", fake)
    yandex.set_on("tok", "d1", True)
    req = fake.requests[0]
    assert req.get_method() == "POST" and req.full_url.endswith("/v1.0/devices/actions")
    assert json.loads(req.data) == {"devices": [{"id": "d1", "actions": [
        {"type": "devices.capabilities.on_off", "state": {"instance": "on", "value": True}}]}]}


def test_device_error_is_raised(monkeypatch):
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen({"status": "ok", "devices": [{"id": "d1", "capabilities": [
        {"type": "devices.capabilities.on_off", "state": {"instance": "on", "action_result": {
            "status": "ERROR", "error_code": "DEVICE_UNREACHABLE", "error_message": "Устройство не в сети"}}}]}]}))
    with pytest.raises(yandex.YandexError, match="Устройство не в сети"):
        yandex.set_on("tok", "d1", True)


@pytest.mark.parametrize("code", [401, 403])
def test_auth_errors(monkeypatch, code):
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen(error=http_error(code)))
    with pytest.raises(yandex.YandexAuthError):
        yandex.list_devices("tok")


def test_network_and_server_errors(monkeypatch):
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen(error=http_error(500)))
    with pytest.raises(yandex.YandexError) as e:
        yandex.list_devices("tok")
    assert not isinstance(e.value, yandex.YandexAuthError)
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen(error=OSError("timeout")))
    with pytest.raises(yandex.YandexError, match="недоступен"):
        yandex.set_on("tok", "d1", False)
