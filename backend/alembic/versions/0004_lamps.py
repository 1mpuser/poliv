"""Лампы как объект: lamps, периоды привязки растений, розетка в Умном доме Яндекса

- lamps: лампа учётки (режим auto/schedule/manual, устройство Яндекса, границы досветки, архив)
- plant_lamps: периоды «растение стояло под лампой»; открытый — не больше одного на растение
- lamp_sessions/lamp_schedules: plant_id → lamp_id; у сессий source (manual/schedule/auto)
- user_settings: зашифрованный токен Яндекса
Старые данные: общая лампа (plant_id NULL) и своя лампа растения становятся объектами-лампами.
Растение со своей лампой получает ещё и закрытый период общей — её часы в истории сохраняются.

Revision ID: 0004
Revises: 0003
Create Date: 2026-09-25
"""
import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision = "0004"
down_revision = "0003"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "lamps",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("user_id", sa.Integer, sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("name", sa.String(100), nullable=False),
        sa.Column("mode", sa.Enum("auto", "schedule", "manual", name="lamp_mode"), nullable=False, server_default="manual"),
        sa.Column("device_id", sa.String(100)),
        sa.Column("device_name", sa.String(200)),
        sa.Column("morning_not_before", sa.Time, nullable=False, server_default="06:00"),
        sa.Column("evening_not_after", sa.Time, nullable=False, server_default="23:00"),
        sa.Column("last_state", sa.Boolean),
        sa.Column("last_error", sa.Text),
        sa.Column("last_error_at", sa.DateTime(timezone=True)),
        sa.Column("archived_at", sa.DateTime(timezone=True)),
        sa.Column("tmp_plant_id", sa.Integer),  # только на время переноса данных
        sa.CheckConstraint("evening_not_after > morning_not_before", name="lamp_bounds_order"),
    )
    op.create_table(
        "plant_lamps",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("lamp_id", sa.Integer, sa.ForeignKey("lamps.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column("ended_at", sa.DateTime(timezone=True)),
        sa.CheckConstraint("ended_at IS NULL OR ended_at >= started_at", name="plant_lamp_end_after_start"),
    )
    op.execute("CREATE UNIQUE INDEX uq_plant_lamp_open ON plant_lamps (plant_id) WHERE ended_at IS NULL")

    op.execute("CREATE TYPE lamp_source AS ENUM ('manual', 'schedule', 'auto')")
    op.add_column("lamp_sessions", sa.Column("lamp_id", sa.Integer))
    op.add_column(
        "lamp_sessions",
        sa.Column("source", postgresql.ENUM(name="lamp_source", create_type=False), nullable=False, server_default="manual"),
    )
    op.add_column("lamp_schedules", sa.Column("lamp_id", sa.Integer))

    # --- перенос данных ---
    has_schedule = "EXISTS (SELECT 1 FROM lamp_schedules s WHERE {cond})"
    op.execute(
        f"""
        INSERT INTO lamps (user_id, name, mode)
        SELECT u.user_id, 'Общая лампа',
               CASE WHEN {has_schedule.format(cond="s.user_id = u.user_id AND s.plant_id IS NULL")}
                    THEN 'schedule' ELSE 'manual' END::lamp_mode
        FROM (SELECT user_id FROM lamp_sessions WHERE plant_id IS NULL
              UNION SELECT user_id FROM lamp_schedules WHERE plant_id IS NULL) u
        """
    )
    op.execute(
        f"""
        INSERT INTO lamps (user_id, name, mode, tmp_plant_id)
        SELECT p.user_id, left('Лампа ' || p.name, 100),
               CASE WHEN {has_schedule.format(cond="s.plant_id = p.id")} THEN 'schedule' ELSE 'manual' END::lamp_mode,
               p.id
        FROM plants p
        WHERE EXISTS (SELECT 1 FROM lamp_sessions x WHERE x.plant_id = p.id)
           OR EXISTS (SELECT 1 FROM lamp_schedules x WHERE x.plant_id = p.id)
        """
    )
    for table in ("lamp_sessions", "lamp_schedules"):
        op.execute(f"UPDATE {table} t SET lamp_id = l.id FROM lamps l WHERE l.tmp_plant_id = t.plant_id")
        op.execute(
            f"UPDATE {table} t SET lamp_id = l.id FROM lamps l "
            "WHERE t.plant_id IS NULL AND l.tmp_plant_id IS NULL AND l.user_id = t.user_id"
        )
    op.execute(
        "UPDATE lamp_sessions SET source = CASE WHEN schedule_id IS NOT NULL THEN 'schedule' ELSE 'manual' END::lamp_source"
    )
    # Период начинается с появления растения, но не позже первой сессии лампы — история целиком
    first = "LEAST(p.added_at, COALESCE((SELECT min(s.started_at) FROM lamp_sessions s WHERE s.lamp_id = l.id), p.added_at))"
    op.execute(
        f"INSERT INTO plant_lamps (plant_id, lamp_id, started_at) "
        f"SELECT p.id, l.id, {first} FROM lamps l JOIN plants p ON p.id = l.tmp_plant_id"
    )
    op.execute(
        f"""
        INSERT INTO plant_lamps (plant_id, lamp_id, started_at, ended_at)
        SELECT p.id, l.id, {first},
               CASE WHEN EXISTS (SELECT 1 FROM lamps o WHERE o.tmp_plant_id = p.id) THEN now() END
        FROM plants p JOIN lamps l ON l.user_id = p.user_id AND l.tmp_plant_id IS NULL
        """
    )

    # --- новая схема сессий и расписаний ---
    op.execute("DROP INDEX uq_lamp_one_open")
    for table in ("lamp_sessions", "lamp_schedules"):
        op.drop_column(table, "plant_id")
        op.alter_column(table, "lamp_id", nullable=False)
        op.create_foreign_key(f"{table}_lamp_id_fkey", table, "lamps", ["lamp_id"], ["id"], ondelete="CASCADE")
        op.create_index(f"ix_{table}_lamp_id", table, ["lamp_id"])
    op.execute("CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (lamp_id) WHERE ended_at IS NULL")
    op.drop_column("lamps", "tmp_plant_id")

    op.add_column("user_settings", sa.Column("yandex_token", sa.Text))
    op.add_column("user_settings", sa.Column("yandex_token_invalid", sa.Boolean, nullable=False, server_default=sa.false()))


def downgrade() -> None:
    op.drop_column("user_settings", "yandex_token_invalid")
    op.drop_column("user_settings", "yandex_token")

    for table in ("lamp_sessions", "lamp_schedules"):
        op.add_column(table, sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), index=True))
    # «Лампа <имя>» с одним растением снова своя лампа растения, остальные — общая
    op.execute(
        """
        CREATE TEMP TABLE own_lamp AS
        SELECT l.id AS lamp_id, min(pl.plant_id) AS plant_id
        FROM lamps l JOIN plant_lamps pl ON pl.lamp_id = l.id AND pl.ended_at IS NULL
        WHERE l.name LIKE 'Лампа %' GROUP BY l.id HAVING count(*) = 1
        """
    )
    for table in ("lamp_sessions", "lamp_schedules"):
        op.execute(f"UPDATE {table} t SET plant_id = o.plant_id FROM own_lamp o WHERE o.lamp_id = t.lamp_id")
    op.execute("DROP TABLE own_lamp")

    op.execute("DROP INDEX uq_lamp_one_open")
    op.execute("UPDATE lamp_sessions SET ended_at = now() WHERE ended_at IS NULL")  # старый индекс — одна на учётку
    op.execute("CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (user_id, COALESCE(plant_id, 0)) WHERE ended_at IS NULL")
    op.drop_column("lamp_sessions", "source")
    op.execute("DROP TYPE lamp_source")
    for table in ("lamp_sessions", "lamp_schedules"):
        op.drop_column(table, "lamp_id")
    op.drop_table("plant_lamps")
    op.drop_table("lamps")
    op.execute("DROP TYPE lamp_mode")
