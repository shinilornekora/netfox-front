import { defineWorkspace } from 'vitest/config';

// unit — слайсы и thunk-и Redux, утилиты, HTTP-клиент, отдельные компоненты.
// integration — всё приложение целиком (роутер + Redux + UI) против
// фейкового gateway на MSW: сценарии ролей клиента, менеджера и админа.
export default defineWorkspace([
  {
    extends: './vite.config.ts',
    test: {
      name: 'unit',
      environment: 'jsdom',
      include: ['src/**/*.test.{ts,tsx}'],
      exclude: ['src/**/*.int.test.{ts,tsx}'],
      setupFiles: ['src/test/setup.ts'],
    },
  },
  {
    extends: './vite.config.ts',
    test: {
      name: 'integration',
      environment: 'jsdom',
      include: ['src/**/*.int.test.{ts,tsx}'],
      setupFiles: ['src/test/setup.ts'],
      testTimeout: 15000,
    },
  },
]);
