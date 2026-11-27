/** Тестовые карты платёжного симулятора Плёнки (billing/internal/adapter/paysim). */
export const cards = [
  { number: '4242424242424242', label: 'Успешно' },
  { number: '4000000000000002', label: 'Банк отклонил' },
  { number: '4000000000009995', label: 'Мало средств' },
  { number: '4000000000000259', label: 'Ответ потерялся' },
] as const;
