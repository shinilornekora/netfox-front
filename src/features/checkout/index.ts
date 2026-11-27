// Публичный API фичи «Оплата»: окно оплаты, кнопка, которая его открывает,
// и событие paid, на которое подписываются другие модели (см. app/listeners).
export { CheckoutModal } from './ui/CheckoutModal';
export { CheckoutButton } from './ui/CheckoutButton';
export { checkoutSlice, opened as checkoutOpened, paid, pollDelay, selectCheckout } from './model';
export type { CheckoutTarget, CheckoutState } from './model';
