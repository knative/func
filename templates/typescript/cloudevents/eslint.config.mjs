import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import eslintPluginPrettierRecommended from 'eslint-plugin-prettier/recommended';

export default [
  { ignores: ['build/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  eslintPluginPrettierRecommended,
  {
    files: ['**/*.ts'],
    rules: {
      'no-console': 'warn',
      'prettier/prettier': ['error', { endOfLine: 'auto' }],
      quotes: ['error', 'single', { allowTemplateLiterals: true }]
    }
  }
];
