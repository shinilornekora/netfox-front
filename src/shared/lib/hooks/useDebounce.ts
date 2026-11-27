import { useDebugValue, useEffect, useState } from 'react';

/**
 * Отложенная копия значения: обновляется, только когда `value` перестаёт
 * меняться на `ms` миллисекунд. Каждое новое значение сбрасывает таймер
 * (функция очистки эффекта), поэтому при быстром наборе срабатывает один раз.
 */
export function useDebounce<V>(value: V, ms: number): V {
  const [debounced, setDebounced] = useState<V>(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), ms);
    return () => clearTimeout(timer);
  }, [value, ms]);
  useDebugValue(debounced);
  return debounced;
}
