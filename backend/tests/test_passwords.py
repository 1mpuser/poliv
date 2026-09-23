from app.passwords import ALPHABET, generate_password, hash_password, verify_password


def test_hash_roundtrip():
    h = hash_password("секрет-123")
    assert h.startswith("scrypt$")
    assert verify_password("секрет-123", h)
    assert not verify_password("секрет-124", h)


def test_same_password_different_salt():
    assert hash_password("x" * 10) != hash_password("x" * 10)


def test_placeholder_hash_never_matches():
    assert not verify_password("", "!")
    assert not verify_password("!", "!")


def test_generated_password():
    p = generate_password()
    assert len(p) == 16
    assert set(p) <= set(ALPHABET)
    assert not set(p) & set("0O1lI")
    assert generate_password() != generate_password()
