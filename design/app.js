/* Поливалка — поведение интерфейса.
   Всё общение с сервером — в объекте api. Остальное — чистый UI. */
(() => {
  'use strict';

  // ---------- API (заменить заглушки на реальные запросы) ----------
  const api = {
    // POST /api/plants/:plantId/events  { type: 'water' | 'feed' | 'lamp_on' | 'lamp_off' }  →  { id }
    async logEvent(plantId, type) {
      return { id: `${plantId}-${type}-${Date.now()}` };
    },
    // DELETE /api/events/:id
    async deleteEvent(eventId) {},
    // PUT /api/settings  (тело — FormData формы настроек)
    async saveSettings(formData) {},
  };

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

  // ---------- Тост с отменой ----------
  const toast = (() => {
    const el = $('.toast');
    if (!el) return { show() {} };
    const text = $('.toast__text', el);
    const undoBtn = $('.toast__undo', el);
    let timer, onUndo;
    undoBtn.addEventListener('click', () => { hide(); onUndo && onUndo(); });
    function hide() { el.classList.remove('is-shown'); clearTimeout(timer); }
    return {
      show(message, undo) {
        text.textContent = message;
        onUndo = undo;
        undoBtn.hidden = !undo;
        el.classList.add('is-shown');
        clearTimeout(timer);
        timer = setTimeout(hide, 5000);
      },
    };
  })();

  // ---------- Статус-плитки ----------
  const FLAGS = { ok: '', soon: 'скоро', late: 'пора' };

  function setStat(card, key, value, status) {
    const stat = $(`.stat[data-stat="${key}"]`, card);
    if (!stat) return null;
    const prev = { value: $('[data-value]', stat).textContent, status: stat.dataset.status, flag: $('[data-flag]', stat).textContent };
    $('[data-value]', stat).textContent = value;
    stat.dataset.status = status;
    $('[data-flag]', stat).textContent = FLAGS[status] ?? '';
    return () => {
      $('[data-value]', stat).textContent = prev.value;
      stat.dataset.status = prev.status;
      $('[data-flag]', stat).textContent = prev.flag;
    };
  }

  // ---------- Быстрые действия ----------
  const MESSAGES = {
    water: 'Полив записан',
    feed: 'Подкормка записана',
    lamp_on: 'Лампа включена',
    lamp_off: 'Лампа выключена',
  };

  function setLamp(btn, on) {
    btn.setAttribute('aria-pressed', String(on));
    const label = $('[data-lamp-label]', btn);
    if (label) label.textContent = on ? 'Лампа горит' : 'Лампа';
  }

  document.addEventListener('click', async (e) => {
    const btn = e.target.closest('.action[data-action]');
    if (!btn || btn.disabled) return;
    const card = btn.closest('[data-plant-id]');
    const plantId = card.dataset.plantId;
    let type = btn.dataset.action;
    let revert = () => {};

    if (type === 'lamp') {
      const on = btn.getAttribute('aria-pressed') !== 'true';
      type = on ? 'lamp_on' : 'lamp_off';
      setLamp(btn, on);
      revert = () => setLamp(btn, !on);
    } else {
      // Оптимистично: «0 дней с полива», статус зелёный
      revert = setStat(card, type, '0', 'ok') || revert;
    }

    btn.classList.add('is-done');
    setTimeout(() => btn.classList.remove('is-done'), 1200);
    if (navigator.vibrate) navigator.vibrate(15);

    btn.disabled = true;
    try {
      const { id } = await api.logEvent(plantId, type);
      toast.show(MESSAGES[type], async () => {
        revert();
        await api.deleteEvent(id);
      });
    } catch (err) {
      revert();
      toast.show('Не удалось записать. Проверьте интернет и нажмите ещё раз.');
    } finally {
      btn.disabled = false;
    }
  });

  // ---------- Фильтр таймлайна ----------
  $$('.chip[data-filter]').forEach((chip) => {
    chip.addEventListener('click', () => {
      const filter = chip.dataset.filter;
      $$('.chip[data-filter]').forEach((c) => c.setAttribute('aria-pressed', String(c === chip)));
      const list = $('[data-timeline]');
      if (!list) return;
      let day = null, dayHasItems = false;
      const closeDay = () => { if (day) day.hidden = !dayHasItems; };
      [...list.children].forEach((li) => {
        if (li.classList.contains('tl-day')) { closeDay(); day = li; dayHasItems = false; return; }
        const show = filter === 'all' || li.dataset.type === filter;
        li.hidden = !show;
        if (show) dayHasItems = true;
      });
      closeDay();
    });
  });

  // ---------- Степперы ----------
  function plural(n, forms) {
    const [one, few, many] = forms;
    const m10 = n % 10, m100 = n % 100;
    if (m10 === 1 && m100 !== 11) return one;
    if (m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20)) return few;
    return many;
  }

  $$('.stepper').forEach((st) => {
    const min = Number(st.dataset.min ?? 0);
    const max = Number(st.dataset.max ?? 99);
    const step = Number(st.dataset.step ?? 1);
    const units = st.dataset.units ? st.dataset.units.split(',') : null;
    const isTime = st.dataset.format === 'time';
    const input = $('input[type="hidden"]', st);
    const spin = $('.stepper__value', st);
    const num = $('.stepper__num', st);
    const unit = $('.stepper__unit', st);
    const [dec, inc] = $$('.stepper__btn', st);

    function render() {
      const v = Number(input.value);
      num.textContent = isTime ? `${String(v).padStart(2, '0')}:00` : v;
      unit.textContent = isTime ? '' : units ? plural(v, units) : '';
      spin.setAttribute('aria-valuenow', v);
      spin.setAttribute('aria-valuemin', min);
      spin.setAttribute('aria-valuemax', max);
      spin.setAttribute('aria-valuetext', num.textContent + (unit.textContent ? ' ' + unit.textContent : ''));
      dec.disabled = v <= min;
      inc.disabled = v >= max;
    }
    function change(dir) {
      const next = Math.min(max, Math.max(min, Number(input.value) + dir * step));
      if (next === Number(input.value)) return false;
      input.value = next;
      input.dispatchEvent(new Event('change', { bubbles: true }));
      render();
      return true;
    }

    // Удержание кнопки — быстрая прокрутка значения
    [dec, inc].forEach((btn) => {
      const dir = Number(btn.dataset.dir);
      let hold, repeat;
      const stop = () => { clearTimeout(hold); clearInterval(repeat); };
      btn.addEventListener('pointerdown', (e) => {
        if (e.button !== 0) return;
        change(dir);
        hold = setTimeout(() => { repeat = setInterval(() => { if (!change(dir)) stop(); }, 90); }, 400);
      });
      ['pointerup', 'pointerleave', 'pointercancel'].forEach((ev) => btn.addEventListener(ev, stop));
      // Клавиатура: Enter/Space на кнопке (pointerdown не сработает)
      btn.addEventListener('click', (e) => { if (e.detail === 0) change(dir); });
    });

    spin.addEventListener('keydown', (e) => {
      const map = { ArrowUp: 1, ArrowRight: 1, ArrowDown: -1, ArrowLeft: -1 };
      if (e.key in map) { e.preventDefault(); change(map[e.key]); }
      if (e.key === 'Home') { e.preventDefault(); input.value = min; render(); }
      if (e.key === 'End') { e.preventDefault(); input.value = max; render(); }
    });

    render();
  });

  // ---------- Тумблеры, которые выключают зависимые поля ----------
  $$('.switch[data-controls]').forEach((sw) => {
    const sync = () => sw.dataset.controls.split(/\s+/).forEach((id) => {
      const f = document.getElementById(id);
      if (f) f.toggleAttribute('data-disabled', !sw.checked);
    });
    sw.addEventListener('change', sync);
    sync();
  });

  // ---------- Сегментированные переключатели ----------
  function getTheme() { try { return localStorage.getItem('theme') || 'system'; } catch { return 'system'; } }
  function applyTheme(t) {
    if (t === 'system') delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = t;
    try { localStorage.setItem('theme', t); } catch {}
  }

  $$('[data-segmented]').forEach((group) => {
    const kind = group.dataset.segmented;
    const buttons = $$('button[data-value]', group);
    const select = (value) => {
      buttons.forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.value === value)));
      if (kind === 'plant') $$('[data-plant-panel]').forEach((p) => { p.hidden = p.dataset.plantPanel !== value; });
      if (kind === 'theme') applyTheme(value);
    };
    buttons.forEach((b) => b.addEventListener('click', () => select(b.dataset.value)));

    if (kind === 'theme') select(getTheme());
    if (kind === 'plant') {
      const fromUrl = new URLSearchParams(location.search).get('plant');
      if (fromUrl && buttons.some((b) => b.dataset.value === fromUrl)) select(fromUrl);
    }
  });

  // ---------- Сохранение настроек ----------
  const form = $('#settings-form');
  if (form) {
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const btn = $('button[type="submit"]', form);
      btn.disabled = true;
      try {
        await api.saveSettings(new FormData(form));
        toast.show('Настройки сохранены');
      } catch {
        toast.show('Не удалось сохранить. Проверьте интернет и попробуйте ещё раз.');
      } finally {
        btn.disabled = false;
      }
    });
  }
})();
