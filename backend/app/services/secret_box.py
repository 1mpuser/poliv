"""Шифрование секретов учётки (токен Яндекса) ключом, выведенным из JWT_SECRET.
Сменили JWT_SECRET — старые токены не расшифруются, их вводят заново."""

import base64

from cryptography.fernet import Fernet, InvalidToken
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.kdf.hkdf import HKDF


def _fernet(secret: str) -> Fernet:
    key = HKDF(algorithm=hashes.SHA256(), length=32, salt=None, info=b"poliv:yandex-token").derive(secret.encode())
    return Fernet(base64.urlsafe_b64encode(key))


def encrypt(plain: str, secret: str) -> str:
    return _fernet(secret).encrypt(plain.encode()).decode()


def decrypt(token: str, secret: str) -> str | None:
    try:
        return _fernet(secret).decrypt(token.encode()).decode()
    except InvalidToken:
        return None
