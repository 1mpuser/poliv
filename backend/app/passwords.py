"""Хэши паролей (scrypt из stdlib) и генерация паролей для выдачи учёток."""

import base64
import hashlib
import hmac
import secrets

# Параметры scrypt: ~16 МБ памяти на проверку — терпимо для 1 vCPU / 2 ГБ
N, R, P = 2**14, 8, 1
MIN_LENGTH, MAX_LENGTH = 8, 128

# Без похожих символов: 0/O, 1/l/I
ALPHABET = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"


def _b64(b: bytes) -> str:
    return base64.b64encode(b).decode()


def hash_password(password: str) -> str:
    salt = secrets.token_bytes(16)
    digest = hashlib.scrypt(password.encode(), salt=salt, n=N, r=R, p=P)
    return f"scrypt${N}${R}${P}${_b64(salt)}${_b64(digest)}"


def verify_password(password: str, stored: str) -> bool:
    try:
        algo, n, r, p, salt, digest = stored.split("$")
    except ValueError:
        return False  # '!' и прочие заглушки — войти нельзя
    if algo != "scrypt":
        return False
    expected = base64.b64decode(digest)
    actual = hashlib.scrypt(
        password.encode(), salt=base64.b64decode(salt), n=int(n), r=int(r), p=int(p), dklen=len(expected)
    )
    return hmac.compare_digest(actual, expected)


def generate_password(length: int = 16) -> str:
    return "".join(secrets.choice(ALPHABET) for _ in range(length))
