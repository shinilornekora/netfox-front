// Публичный API фичи «Вход и регистрация»: формы, кнопка выхода и слайс
// с ошибками и состоянием «идёт запрос». Сами запросы (login/register/logout)
// — в сущности session: от них зависит, кто сейчас в приложении.
export { LoginForm } from './ui/LoginForm';
export { RegisterForm } from './ui/RegisterForm';
export { LogoutButton } from './ui/LogoutButton';
export { authSlice } from './model';
export type { AuthFormState } from './model';
