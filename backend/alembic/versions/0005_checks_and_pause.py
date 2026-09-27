"""Проверка грунта и пауза лампы

- soil_checks: «хозяин потрогал грунт» — счётчик растения сбрасывается, как от полива
- lamps.paused_until: до какого момента лампа на паузе (гаснет и сама перезагорится)
- lamp_sessions.after_pause: сессия — хвост после паузы; replan_auto её не удаляет
Частичные индексы uq_plant_lamp_open и uq_lamp_one_open остаются как были.

Revision ID: 0005
Revises: 0004
Create Date: 2026-09-26
"""
import sqlalchemy as sa
from alembic import op

revision = "0005"
down_revision = "0004"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "soil_checks",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("checked_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
    )
    op.add_column("lamps", sa.Column("paused_until", sa.DateTime(timezone=True)))
    op.add_column("lamp_sessions", sa.Column("after_pause", sa.Boolean, nullable=False, server_default=sa.false()))


def downgrade() -> None:
    op.drop_column("lamp_sessions", "after_pause")
    op.drop_column("lamps", "paused_until")
    op.drop_table("soil_checks")
