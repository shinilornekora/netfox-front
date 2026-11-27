// Публичный API сущности «Сессия»: кто вошёл и с какой ролью.
export {
  sessionSlice,
  restoreSession,
  login,
  register,
  logout,
  sessionLost,
  userUpdated,
  selectStatus,
  selectUser,
  selectRole,
  selectIsAuthenticated,
  hasRole,
} from './model';
export type { AuthStatus, SessionState } from './model';
