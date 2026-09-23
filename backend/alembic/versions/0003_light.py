"""Свет: город учётки, световой день по дням, расписания ламп, норма света растения

- plants.lamp_hours_per_day → light_target_hours: норма ВСЕГО света (солнце + лампа), значение сохраняется
- user_settings: город (название и координаты)
- daylight_days: восход, закат, световой день и солнечные часы по дням (Open-Meteo)
- lamp_schedules: интервалы программируемой розетки; plant_id NULL — общая лампа
- lamp_sessions.schedule_id: сессия создана по расписанию

Revision ID: 0003
Revises: 0002
Create Date: 2026-09-24
"""
import sqlalchemy as sa
from alembic import op

revision = "0003"
down_revision = "0002"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.alter_column("plants", "lamp_hours_per_day", new_column_name="light_target_hours")
    op.execute("ALTER TABLE plants RENAME CONSTRAINT lamp_hours_range TO light_target_range")

    op.add_column("user_settings", sa.Column("location_name", sa.String(200)))
    op.add_column("user_settings", sa.Column("latitude", sa.Float))
    op.add_column("user_settings", sa.Column("longitude", sa.Float))

    op.create_table(
        "daylight_days",
        sa.Column("user_id", sa.Integer, sa.ForeignKey("users.id", ondelete="CASCADE"), primary_key=True),
        sa.Column("day", sa.Date, primary_key=True),
        sa.Column("sunrise", sa.DateTime(timezone=True)),
        sa.Column("sunset", sa.DateTime(timezone=True)),
        sa.Column("daylight_hours", sa.Float, nullable=False),
        sa.Column("sunshine_hours", sa.Float, nullable=False),
        sa.Column("fetched_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
    )

    op.create_table(
        "lamp_schedules",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("user_id", sa.Integer, sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), index=True),
        sa.Column("start_time", sa.Time, nullable=False),
        sa.Column("end_time", sa.Time, nullable=False),
        sa.CheckConstraint("end_time > start_time", name="schedule_end_after_start"),
    )

    op.add_column(
        "lamp_sessions",
        sa.Column("schedule_id", sa.Integer, sa.ForeignKey("lamp_schedules.id", ondelete="SET NULL"), index=True),
    )


def downgrade() -> None:
    op.drop_column("lamp_sessions", "schedule_id")
    op.drop_table("lamp_schedules")
    op.drop_table("daylight_days")
    for col in ("longitude", "latitude", "location_name"):
        op.drop_column("user_settings", col)
    op.execute("ALTER TABLE plants RENAME CONSTRAINT light_target_range TO lamp_hours_range")
    op.alter_column("plants", "light_target_hours", new_column_name="lamp_hours_per_day")
