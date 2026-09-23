/** Без похожих символов: 0/O, 1/l/I — пароль часто диктуют или переписывают руками */
export const PASSWORD_ALPHABET = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';

export function generatePassword(length = 16): string {
  const out: string[] = [];
  const buf = new Uint32Array(1);
  // Отбраковка вместо остатка от деления — без перекоса в пользу первых символов
  const limit = Math.floor(0x1_0000_0000 / PASSWORD_ALPHABET.length) * PASSWORD_ALPHABET.length;
  while (out.length < length) {
    crypto.getRandomValues(buf);
    if (buf[0] < limit) out.push(PASSWORD_ALPHABET[buf[0] % PASSWORD_ALPHABET.length]);
  }
  return out.join('');
}
