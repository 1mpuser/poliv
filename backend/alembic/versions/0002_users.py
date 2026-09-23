"""Мультиучётки: пользователи, данные и настройки per-user

Существующие данные переходят к учётке-заглушке id=1 (owner@localhost.invalid, админ,
пароль не задан — войти нельзя). Настоящую почту и пароль ей даёт
`python -m app.cli set-owner --email ...`.

Revision ID: 0002
Revises: 0001
Create Date: 2026-09-24
"""
import sqlalchemy as sa
from alembic import op

revision = "0002"
down_revision = "0001"
branch_labels = None
depends_on = None

OWNED = ("plants", "fertilizer_types", "lamp_sessions")


def upgrade() -> None:
    op.create_table(
        "users",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("email", sa.String(254), nullable=False, unique=True),
        # '!' — не хэш: по такому паролю войти невозможно
        sa.Column("password_hash", sa.String(255), nullable=False),
        sa.Column("is_admin", sa.Boolean, nullable=False, server_default=sa.false()),
        sa.Column("blocked_at", sa.DateTime(timezone=True)),
        # Растёт при смене пароля и блокировке — старые JWT перестают действовать
        sa.Column("token_version", sa.Integer, nullable=False, server_default="0"),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
    )
    op.execute("INSERT INTO users (id, email, password_hash, is_admin) VALUES (1, 'owner@localhost.invalid', '!', true)")
    op.execute("SELECT setval('users_id_seq', 1)")

    for table in OWNED:
        op.add_column(table, sa.Column("user_id", sa.Integer))
        op.execute(f"UPDATE {table} SET user_id = 1")
        op.alter_column(table, "user_id", nullable=False)
        op.create_foreign_key(f"{table}_user_id_fkey", table, "users", ["user_id"], ["id"], ondelete="CASCADE")
        op.create_index(f"ix_{table}_user_id", table, ["user_id"])

    # Названия удобрений уникальны в пределах учётки
    op.drop_constraint("fertilizer_types_name_key", "fertilizer_types", type_="unique")
    op.create_unique_constraint("uq_fertilizer_user_name", "fertilizer_types", ["user_id", "name"])

    # Одна горящая лампа на растение и одна общая — на учётку
    op.execute("DROP INDEX uq_lamp_one_open")
    op.execute(
        "CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (user_id, COALESCE(plant_id, 0)) "
        "WHERE ended_at IS NULL"
    )

    # Глобальная строка настроек (id=1) становится настройками учётки 1
    op.drop_constraint("single_row", "app_settings", type_="check")
    op.alter_column("app_settings", "id", new_column_name="user_id", server_default=None)
    op.rename_table("app_settings", "user_settings")
    op.execute("ALTER INDEX app_settings_pkey RENAME TO user_settings_pkey")
    op.create_foreign_key("user_settings_user_id_fkey", "user_settings", "users", ["user_id"], ["id"], ondelete="CASCADE")


def downgrade() -> None:
    # Обратно в однопользовательский режим: остаются только данные учётки 1
    for table in OWNED:
        op.execute(f"DELETE FROM {table} WHERE user_id <> 1")
    op.execute("DELETE FROM user_settings WHERE user_id <> 1")

    op.drop_constraint("user_settings_user_id_fkey", "user_settings", type_="foreignkey")
    op.execute("ALTER INDEX user_settings_pkey RENAME TO app_settings_pkey")
    op.rename_table("user_settings", "app_settings")
    op.alter_column("app_settings", "user_id", new_column_name="id", server_default="1")
    op.create_check_constraint("single_row", "app_settings", "id = 1")

    op.execute("DROP INDEX uq_lamp_one_open")
    op.execute("CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (COALESCE(plant_id, 0)) WHERE ended_at IS NULL")
    op.drop_constraint("uq_fertilizer_user_name", "fertilizer_types", type_="unique")
    op.create_unique_constraint("fertilizer_types_name_key", "fertilizer_types", ["name"])

    for table in OWNED:
        op.drop_index(f"ix_{table}_user_id", table)
        op.drop_constraint(f"{table}_user_id_fkey", table, type_="foreignkey")
        op.drop_column(table, "user_id")
    op.drop_table("users")
