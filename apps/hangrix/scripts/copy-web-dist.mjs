// Copies the Nuxt static-generated output into internal/web/dist so that the
// Go binary can pick it up via //go:embed at build time.
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const src = resolve(here, '../../web/.output/public')
const dst = resolve(here, '../internal/web/dist')

mkdirSync(dst, { recursive: true })
for (const entry of readdirSync(dst)) {
  rmSync(join(dst, entry), { recursive: true, force: true })
}

if (existsSync(src)) {
  cpSync(src, dst, { recursive: true })
  patchHtmlEntry(join(dst, 'index.html'))
  patchHtmlEntry(join(dst, '200.html'))
  console.log(`copied ${src} -> ${dst}`)
} else {
  console.warn(`source not found: ${src} (skipping copy)`)
}

writeFileSync(join(dst, '.gitkeep'), '')

function patchHtmlEntry(file) {
  if (!existsSync(file)) {
    return
  }
  let html = readFileSync(file, 'utf8')
  html = html.replace(/<link rel="modulepreload"[^>]*>\s*/g, '')
  html = html.replace(
    /<script type="module" src="([^"]+)" crossorigin><\/script>/,
    (_, srcPath) => `<script type="module">
const __hangrixEntry = ${JSON.stringify(srcPath)};
const __hangrixDiag = (reason, error) => {
  const body = document.body || document.documentElement;
  if (!body || document.getElementById('__hangrix_diag')) return;
  const pre = document.createElement('pre');
  pre.id = '__hangrix_diag';
  pre.style.cssText = 'position:fixed;z-index:2147483647;inset:0;margin:0;padding:14px;overflow:auto;background:#111827;color:#f9fafb;font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:pre-wrap;word-break:break-word;';
  const scripts = [].map.call(document.scripts, (s) => s.src || '[inline]').join('\\n');
  const links = [].map.call(document.querySelectorAll('link[href]'), (s) => s.rel + ': ' + s.href).join('\\n');
  const details = error && (error.stack || error.message || String(error)) || '';
  pre.textContent =
    'Hangrix mobile diagnostic\\n' +
    'reason: ' + reason + '\\n' +
    'url: ' + location.href + '\\n' +
    'ua: ' + navigator.userAgent + '\\n' +
    'error: ' + details + '\\n\\n' +
    'scripts:\\n' + scripts + '\\n\\n' +
    'links:\\n' + links;
  body.appendChild(pre);
};
const __hangrixMaybeRecover = (error) => {
  const details = String(error && (error.stack || error.message || error) || '').toLowerCase();
  if (!details.includes('dynamically imported module') &&
      !details.includes('importing a module script failed') &&
      !details.includes('failed to fetch') &&
      !details.includes('unexpected token')) {
    return false;
  }
  if (sessionStorage.getItem('__hangrix_chunk_retry') === '1') {
    return false;
  }
  sessionStorage.setItem('__hangrix_chunk_retry', '1');
  location.replace(location.pathname + location.search + (location.search ? '&' : '?') + 'v=' + Date.now());
  return true;
};
window.addEventListener('pageshow', () => {
  try {
    sessionStorage.removeItem('__hangrix_chunk_retry');
  } catch {}
});
import(__hangrixEntry).catch((error) => {
  try {
    if (Array.isArray(window.__hangrixDiagLogs)) {
      window.__hangrixDiagLogs.push({ t: 'module', m: String(error && (error.stack || error.message || error)), at: new Date().toISOString() });
    }
  } catch {}
  if (__hangrixMaybeRecover(error)) {
    return;
  }
  __hangrixDiag('module-error', error);
});
</script>`,
  )
  writeFileSync(file, html)
}
