/**
 * Правило «розетка не в нужном состоянии» для лампы.
 * Одно и то же на главной, в карточке растения и в списке ламп.
 * Бэкенд отдаёт краткую сводку лампы (`has_device`) и полную (`device_id`).
 */

/** Минимум полей, по которому можно судить о расхождении плана и факта. */
export interface LampIssueSource {
  /** Есть ли физическая розетка (краткая сводка лампы растения). */
  has_device?: boolean;
  /** ID розетки (полная лампа) — непустой означает, что розетка есть. */
  device_id?: string | null;
  /** Ошибка последней команды к розетке. */
  last_error?: string | null;
}

function hasPlug(l: LampIssueSource): boolean {
  return l.has_device === true || Boolean(l.device_id);
}

/**
 * Текст ошибки, если «розетка не в нужном состоянии»: у лампы есть розетка
 * и записанный `last_error`. Иначе — `null`. Это и есть единое правило.
 */
export function plugIssue(l: LampIssueSource | null | undefined): string | null {
  if (!l) return null;
  if (!hasPlug(l)) return null;
  return l.last_error || null;
}

/** Подпись плашки/чипа лампы: обычная или «должна гореть/быть выключена». */
export function lampChipText(l: { name: string; is_on: boolean } & LampIssueSource): string {
  if (plugIssue(l)) {
    return l.is_on
      ? `${l.name} должна гореть — розетка не отвечает`
      : `${l.name} должна быть выключена — розетка не отвечает`;
  }
  return l.is_on ? `${l.name} горит` : l.name;
}
