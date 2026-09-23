"""Начальная схема и стартовые данные: лимон, лайм, два удобрения

Revision ID: 0001
Revises:
Create Date: 2026-09-23
"""
import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision = "0001"
down_revision = None
branch_labels = None
depends_on = None

season = postgresql.ENUM("active", "dormant", name="season", create_type=False)
feed_method = postgresql.ENUM("root", "foliar", name="feed_method", create_type=False)


def ts(name: str, nullable: bool = False) -> sa.Column:
    return sa.Column(
        name,
        sa.DateTime(timezone=True),
        nullable=nullable,
        server_default=None if nullable else sa.func.now(),
    )


def upgrade() -> None:
    season.create(op.get_bind(), checkfirst=True)
    feed_method.create(op.get_bind(), checkfirst=True)

    op.create_table(
        "plants",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("name", sa.String(100), nullable=False),
        sa.Column("species", sa.String(100), nullable=False, server_default=""),
        sa.Column("location", sa.String(100)),
        sa.Column("pot_size_l", sa.Float),
        ts("added_at"),
        sa.Column("notes", sa.Text),
        sa.Column("water_interval_days", sa.Integer, nullable=False, server_default="4"),
        sa.Column("fertilizing_enabled", sa.Boolean, nullable=False, server_default=sa.true()),
        sa.Column("lamp_hours_per_day", sa.Float, nullable=False, server_default="12"),
        sa.Column("repot_check_interval_months", sa.Integer, nullable=False, server_default="12"),
        sa.CheckConstraint("water_interval_days > 0", name="water_interval_positive"),
        sa.CheckConstraint("lamp_hours_per_day BETWEEN 0 AND 24", name="lamp_hours_range"),
        sa.CheckConstraint("repot_check_interval_months > 0", name="repot_interval_positive"),
    )

    op.create_table(
        "fertilizer_types",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("name", sa.String(100), nullable=False, unique=True),
        sa.Column("npk", sa.String(50), nullable=False, server_default=""),
        sa.Column("root_dose_ml_per_l", sa.Float),
        sa.Column("foliar_dose_ml_per_l", sa.Float),
        sa.Column("interval_days_active_season", sa.Integer, nullable=False),
        sa.Column("interval_days_dormant_season", sa.Integer),
    )

    op.create_table(
        "watering_logs",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), nullable=False, index=True),
        ts("watered_at"),
        sa.Column("note", sa.Text),
    )

    op.create_table(
        "feeding_logs",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("fertilizer_type_id", sa.Integer, sa.ForeignKey("fertilizer_types.id", ondelete="SET NULL")),
        sa.Column("method", feed_method, nullable=False),
        ts("fed_at"),
        sa.Column("note", sa.Text),
    )

    op.create_table(
        "lamp_sessions",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), index=True),
        ts("started_at"),
        ts("ended_at", nullable=True),
        sa.Column("planned_hours_per_day", sa.Float, nullable=False, server_default="12"),
        sa.CheckConstraint("ended_at IS NULL OR ended_at >= started_at", name="lamp_end_after_start"),
    )
    # Одна горящая сессия на растение и одна для общей лампы (plant_id NULL → 0)
    op.execute(
        "CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (COALESCE(plant_id, 0)) WHERE ended_at IS NULL"
    )

    op.create_table(
        "repotting_logs",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), nullable=False, index=True),
        ts("repotted_at"),
        sa.Column("pot_size_before", sa.Float),
        sa.Column("pot_size_after", sa.Float),
        sa.Column("note", sa.Text),
    )

    op.create_table(
        "app_settings",
        sa.Column("id", sa.Integer, primary_key=True, server_default="1"),
        sa.Column("current_season", season, nullable=False, server_default="active"),
        sa.Column("notify_days_ahead", sa.Integer, nullable=False, server_default="1"),
        sa.CheckConstraint("id = 1", name="single_row"),
    )

    # ---------- Стартовые данные ----------
    op.execute("INSERT INTO app_settings (id) VALUES (1)")
    op.execute(
        "INSERT INTO plants (name, species, water_interval_days, lamp_hours_per_day) VALUES "
        "('Лимон', 'Лимон Мейера', 4, 12), ('Лайм', 'Лайм', 3, 12)"
    )
    # NPK и дозы не заполнены намеренно — впишите с этикетки в «Настройках»
    op.execute(
        "INSERT INTO fertilizer_types (name, interval_days_active_season, interval_days_dormant_season) VALUES "
        "('Lomonosoff', 14, 30), ('Bona Forte', 14, 30)"
    )


def downgrade() -> None:
    for table in (
        "app_settings",
        "repotting_logs",
        "lamp_sessions",
        "feeding_logs",
        "watering_logs",
        "fertilizer_types",
        "plants",
    ):
        op.drop_table(table)
    feed_method.drop(op.get_bind(), checkfirst=True)
    season.drop(op.get_bind(), checkfirst=True)
