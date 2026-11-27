import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import prettier from 'eslint-config-prettier';
import globals from 'globals';

// Feature-Sliced Design: слой может импортировать только слои ниже себя
// (app → pages → widgets → features → entities → shared), соседние слайсы
// одного слоя друг друга не видят, а в чужой слайс входят только через его
// публичный API (index.ts): '@/features/checkout', но не '@/features/checkout/model'.
const LAYERS = ['app', 'pages', 'widgets', 'features', 'entities', 'shared'];
const SLICED = ['pages', 'widgets', 'features', 'entities'];

function fsdRule(layer) {
  const above = LAYERS.slice(0, LAYERS.indexOf(layer));
  const patterns = [
    {
      group: SLICED.map((l) => `@/${l}/*/**`),
      message: 'FSD: импорт из чужого слайса — только через его публичный API (index.ts).',
    },
  ];
  if (above.length)
    patterns.push({
      group: above.map((l) => `@/${l}/**`),
      // RootState/AppDispatch собираются в app, а shared/lib/redux берёт их только как типы.
      allowTypeImports: true,
      message: `FSD: слой ${layer} не может импортировать слои выше себя (${above.join(', ')}).`,
    });
  if (SLICED.includes(layer))
    patterns.push({
      group: [`@/${layer}/*`],
      message: `FSD: слайсы слоя ${layer} не импортируют друг друга; свой слайс — относительным путём.`,
    });
  return {
    files: [`src/${layer}/**/*.{ts,tsx}`],
    // Юнит-тесты собирают настоящий стор (makeStore из app) — им можно; публичный API — всё равно обязателен.
    ignores: ['src/**/*.test.{ts,tsx}'],
    rules: { '@typescript-eslint/no-restricted-imports': ['error', { patterns }] },
  };
}

export default tseslint.config(
  {
    ignores: [
      'dist/**',
      'coverage/**',
      'node_modules/**',
      'public/mockServiceWorker.js',
      'backend/**',
      'docs/**',
      'scripts/**',
      'deploy/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: { globals: globals.browser },
    plugins: { react, 'react-hooks': reactHooks },
    settings: { react: { version: 'detect' } },
    rules: {
      ...react.configs.recommended.rules,
      ...react.configs['jsx-runtime'].rules,
      ...reactHooks.configs.recommended.rules,
      'react/prop-types': 'off',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      '@typescript-eslint/consistent-type-imports': 'error',
      'no-console': ['warn', { allow: ['warn', 'error'] }],
    },
  },
  {
    // Тесты, демо и точка входа — вне слоёв, но в слайсы тоже входят через публичный API.
    files: ['src/**/*.{ts,tsx}'],
    rules: {
      '@typescript-eslint/no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: SLICED.map((l) => `@/${l}/*/**`),
              message: 'FSD: импорт из чужого слайса — только через его публичный API (index.ts).',
            },
          ],
        },
      ],
    },
  },
  ...LAYERS.map(fsdRule),
  {
    files: ['webpack/**/*.cjs'],
    languageOptions: { globals: globals.node, sourceType: 'commonjs' },
    rules: { '@typescript-eslint/no-require-imports': 'off' },
  },
  prettier,
);
