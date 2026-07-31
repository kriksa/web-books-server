import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'path';
import { fileURLToPath } from 'url';
import { execSync } from 'child_process';
import { readFileSync } from 'fs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');

function resolveAppVersion() {
  try {
    const tag = execSync('git describe --tags --exact-match', {
      cwd: repoRoot,
      stdio: ['ignore', 'pipe', 'ignore'],
      encoding: 'utf8',
    }).trim();
    if (tag) return tag;
  } catch {
    // no exact tag for current commit
  }

  try {
    const pkgRaw = readFileSync(path.join(__dirname, 'package.json'), 'utf8');
    const pkg = JSON.parse(pkgRaw);
    if (typeof pkg.version === 'string' && pkg.version.trim()) {
      return `v${pkg.version.trim()}`;
    }
  } catch {
    // package.json unreadable or missing version
  }

  try {
    const describe = execSync('git describe --tags --always --dirty', {
      cwd: repoRoot,
      stdio: ['ignore', 'pipe', 'ignore'],
      encoding: 'utf8',
    }).trim();
    if (describe) return describe;
  } catch {
    // git not available
  }

  return 'dev';
}

const appVersion = resolveAppVersion();

export default defineConfig({
  plugins: [vue()],
  root: __dirname,
  publicDir: 'public',
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  resolve: {
    alias: {
      '@': __dirname,
    },
  },
  build: {
    outDir: path.join(repoRoot, 'internal', 'spaembed', 'spa'),
    emptyOutDir: true,
    rollupOptions: {
      input: {
        main: path.join(__dirname, 'index.html'),
        reader: path.join(__dirname, 'reader.html'),
      },
    },
  },
});
