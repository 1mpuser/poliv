from app.services.secret_box import decrypt, encrypt


def test_roundtrip_and_ciphertext_hides_token():
    box = encrypt("y0_secret-token", "jwt-secret")
    assert "y0_secret" not in box
    assert decrypt(box, "jwt-secret") == "y0_secret-token"


def test_other_secret_or_garbage_gives_none():
    box = encrypt("y0_secret-token", "jwt-secret")
    assert decrypt(box, "другой секрет") is None
    assert decrypt("мусор", "jwt-secret") is None
