export type Obj = Record<string, any>;
export function checkCounter(n: unknown): number {
  if (typeof n !== 'number' || !Number.isSafeInteger(n) || n < 0) throw new Error('Небезопасное значение последовательности');
  return n;
}
export async function get(path: string): Promise<Obj> {
  const response = await fetch(path, {headers: {'Accept': 'application/json'}});
  if (!response.ok) throw new Error('Не удалось получить данные. Повторите запрос.');
  const body = await response.json();
  if (!body || typeof body !== 'object' || Array.isArray(body)) throw new Error('Некорректный ответ сервера');
  for (const key of ['sequence', 'view_sequence', 'next_after_sequence']) if (body[key] != null) checkCounter(body[key]);
  if (Array.isArray(body.items)) for (const item of body.items) checkCounter(item.sequence);
  return body;
}
