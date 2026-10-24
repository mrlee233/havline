import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import tseslint from 'typescript-eslint'

// 说明：这份配置刻意只开「低争议」规则。
// 项目里已有 2.7 万行前端代码，一次性打开 recommended 全量会砸出几千条告警，
// 反而没人看。先把「未使用变量 / 未定义变量 / == 与 ===」这类真问题卡住，
// 再逐步加规则。vite.config.ts 里已经在 build 前跑 vue-tsc，类型问题由它兜。
export default tseslint.config(
  {
    ignores: ['dist/**', 'node_modules/**', 'src/**/*.d.ts'],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/essential'],
  {
    files: ['**/*.{ts,vue}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      // Vue SFC 的 <script setup lang="ts"> 必须显式指定 TS 解析器，
      // 否则 eslint-plugin-vue 用的默认解析器不认识类型标注，会报
      // 「Parsing error: Unexpected token XxxType」。
      parserOptions: {
        parser: tseslint.parser,
        extraFileExtensions: ['.vue'],
        ecmaFeatures: { jsx: false },
      },
      globals: {
        // 浏览器环境
        window: 'readonly',
        document: 'readonly',
        navigator: 'readonly',
        localStorage: 'readonly',
        sessionStorage: 'readonly',
        fetch: 'readonly',
        FormData: 'readonly',
        Blob: 'readonly',
        URL: 'readonly',
        EventSource: 'readonly',
        WebSocket: 'readonly',
        ResizeObserver: 'readonly',
        MutationObserver: 'readonly',
        HTMLElement: 'readonly',
        HTMLInputElement: 'readonly',
        HTMLTextAreaElement: 'readonly',
        HTMLDivElement: 'readonly',
        HTMLCanvasElement: 'readonly',
        Node: 'readonly',
        Element: 'readonly',
        Event: 'readonly',
        KeyboardEvent: 'readonly',
        MouseEvent: 'readonly',
        AbortController: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
        setInterval: 'readonly',
        clearInterval: 'readonly',
        requestAnimationFrame: 'readonly',
        cancelAnimationFrame: 'readonly',
        matchMedia: 'readonly',
        console: 'readonly',
        crypto: 'readonly',
        performance: 'readonly',
        // 构建期注入
        __APP_VERSION__: 'readonly',
      },
    },
    rules: {
      // 真问题：未使用变量、未定义变量
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrors: 'none',
        },
      ],
      // 宽松处理 any：现有 API 类型里有历史遗留，先不拦
      '@typescript-eslint/no-explicit-any': 'off',
      // 空块与空接口在这类前端代码里常用于「占位」，不视为错误
      'no-empty': ['error', { allowEmptyCatch: true }],
      // Vue：多词组件名不强求（现有单文件组件已大量单词名）
      'vue/multi-word-component-names': 'off',
      // 未使用组件显式注册检查交由 vue-tsc
      'vue/no-unused-components': 'off',
      // TaskProviderForm / LogsView 里的 form、autoRefresh 都是 v-model 双向绑定的 prop。
      // 这是 Vue 官方推荐写法（父组件传 ref，子组件 v-model 直接改），规则默认会误报。
      'vue/no-mutating-props': ['error', { shallowOnly: true }],
    },
  },
  {
    // 配置文件与脚本允许 CommonJS 风格的环境
    files: ['*.config.{ts,js}', 'vite.config.ts'],
    languageOptions: {
      globals: {
        process: 'readonly',
        __dirname: 'readonly',
      },
    },
  },
)
