import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import fs from 'node:fs';
import path from 'node:path';

const outDir = path.resolve(__dirname, '../web/dist');
const BACKEND_TARGET = process.env.QD_API || 'http://127.0.0.1:48120';
const BACKEND_WS = BACKEND_TARGET.replace(/^http/, 'ws');

const ES_TOOLKIT_COMPAT_DIRS = ['array', 'function', 'math', 'object', 'predicate', 'string', 'util'];
const ES_TOOLKIT_SHIM_PREFIX = '\0es-toolkit-compat:';

function findEsToolkitCompatMjs(name) {
  for (const sub of ES_TOOLKIT_COMPAT_DIRS) {
    const candidate = path.resolve(__dirname, 'node_modules/es-toolkit/dist/compat', sub, `${name}.mjs`);
    if (fs.existsSync(candidate)) return candidate;
  }
  return null;
}

function esToolkitCompatEsmResolver() {
  return {
    name: 'es-toolkit-compat-esm',
    enforce: 'pre',
    resolveId(id) {
      const m = id.match(/^es-toolkit\/compat\/(.+)$/);
      if (!m) return null;
      if (!findEsToolkitCompatMjs(m[1])) return null;
      return ES_TOOLKIT_SHIM_PREFIX + m[1];
    },
    load(id) {
      if (!id.startsWith(ES_TOOLKIT_SHIM_PREFIX)) return null;
      const name = id.slice(ES_TOOLKIT_SHIM_PREFIX.length);
      const target = findEsToolkitCompatMjs(name);
      if (!target) return null;
      const url = target.replace(/\\/g, '/');
      return `import { ${name} } from ${JSON.stringify(url)};\nexport { ${name} };\nexport default ${name};\n`;
    },
  };
}

const backend = { target: BACKEND_TARGET, changeOrigin: true, secure: false };

export default defineConfig({
  plugins: [esToolkitCompatEsmResolver(), react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  optimizeDeps: {
    rolldownOptions: {
      plugins: [esToolkitCompatEsmResolver()],
    },
  },
  build: {
    outDir,
    emptyOutDir: true,
    sourcemap: Boolean(process.env.QD_MAPS),
    target: 'es2020',
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      input: {
        index: path.resolve(__dirname, 'index.html'),
      },
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined;
          if (id.includes('/@ant-design/icons/') || id.includes('/@ant-design/icons-svg/')) return 'vendor-icons';
          if (
            id.includes('/node_modules/react-i18next/')
            || id.includes('/node_modules/i18next/')
          ) return 'vendor-i18next';
          if (
            id.includes('/node_modules/react/')
            || id.includes('/node_modules/react-dom/')
            || id.includes('/node_modules/scheduler/')
          ) return 'vendor-react';
          if (id.includes('/node_modules/@tanstack/')) return 'vendor-tanstack';
          if (id.includes('/node_modules/react-router')) return 'vendor-router';
          if (
            id.includes('/node_modules/swagger-ui-react/')
            || id.includes('/node_modules/swagger-ui/')
            || id.includes('/node_modules/swagger-client/')
          ) return 'vendor-swagger';
          if (
            id.includes('/node_modules/recharts/')
            || id.includes('/node_modules/victory-vendor/')
            || id.includes('/node_modules/d3-')
          ) return 'vendor-recharts';
          if (id.includes('dayjs')) return 'vendor-dayjs';
          if (id.includes('axios')) return 'vendor-axios';
          return 'vendor';
        },
      },
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '^/panel/(api|setting)(/|$)': backend,
      '^/client/api(/|$)': backend,
      '^/theme\\.json$': backend,
      '^/ws$': { ...backend, target: BACKEND_WS, ws: true },
    },
  },
});
