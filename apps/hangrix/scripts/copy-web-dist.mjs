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
const __hangrixEscape = (value) => String(value == null ? '' : value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
const __hangrixUrlWith = (params) => {
  const url = new URL(location.href);
  Object.keys(params).forEach((key) => {
    if (params[key] == null) url.searchParams.delete(key);
    else url.searchParams.set(key, params[key]);
  });
  return url.toString();
};
const __hangrixLoadFailure = (reason, error) => {
  const body = document.body || document.documentElement;
  if (!body || document.getElementById('__hangrix_load_failure')) return;
  const scripts = [].map.call(document.scripts, (s) => s.src || '[inline]').join('\\n');
  const links = [].map.call(document.querySelectorAll('link[href]'), (s) => s.rel + ': ' + s.href).join('\\n');
  const details = error && (error.stack || error.message || String(error)) || '';
  const debug = new URLSearchParams(location.search).has('hangrix_diag')
    ? '<pre style="margin:14px 0 0;max-height:180px;overflow:auto;border:1px solid #27272a;border-radius:8px;padding:10px;background:#09090b;color:#d4d4d8;font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:pre-wrap;word-break:break-word;">reason: ' + __hangrixEscape(reason) + '\\nurl: ' + __hangrixEscape(location.href) + '\\nua: ' + __hangrixEscape(navigator.userAgent) + '\\nerror: ' + __hangrixEscape(details) + '\\n\\nscripts:\\n' + __hangrixEscape(scripts) + '\\n\\nlinks:\\n' + __hangrixEscape(links) + '</pre>'
    : '';
  const panel = document.createElement('div');
  panel.id = '__hangrix_load_failure';
  panel.style.cssText = 'position:fixed;z-index:2147483647;inset:0;display:flex;align-items:center;justify-content:center;padding:20px;background:#09090b;color:#fafafa;font:14px/1.5 system-ui,-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;';
  panel.innerHTML = '<div style="width:min(520px,100%);border:1px solid #27272a;border-radius:12px;background:#111113;padding:20px;box-shadow:0 20px 60px rgba(0,0,0,.35)"><h1 style="margin:0 0 8px;font-size:20px;line-height:1.25">Hangrix 页面加载失败</h1><p style="margin:0 0 16px;color:#a1a1aa">页面资源可能还在更新或网络请求被中断。先刷新一次；如果仍失败，可以临时打开轻量版。</p><div style="display:grid;gap:10px;grid-template-columns:repeat(auto-fit,minmax(120px,1fr))"><button id="__hangrix_retry" style="height:40px;border:1px solid #fafafa;border-radius:8px;background:#fafafa;color:#09090b;font-weight:650;cursor:pointer">重新加载</button><a style="height:40px;border:1px solid #3f3f46;border-radius:8px;color:#fafafa;text-decoration:none;display:flex;align-items:center;justify-content:center" href="' + __hangrixEscape(__hangrixUrlWith({ desktop: '1', mobile: null, v: Date.now() })) + '">强制桌面版</a><a style="height:40px;border:1px solid #3f3f46;border-radius:8px;color:#fafafa;text-decoration:none;display:flex;align-items:center;justify-content:center" href="' + __hangrixEscape(__hangrixUrlWith({ mobile: '1', desktop: null, v: Date.now() })) + '">打开轻量版</a></div>' + debug + '</div>';
  body.appendChild(panel);
  const retry = document.getElementById('__hangrix_retry');
  if (retry) retry.onclick = () => location.replace(__hangrixUrlWith({ v: Date.now() }));
};
const __hangrixMaybeRecover = (error) => {
  const details = String(error && (error.stack || error.message || error) || '').toLowerCase();
  if (!details.includes('dynamically imported module') &&
      !details.includes('importing a module script failed') &&
      !details.includes('failed to fetch') &&
      !details.includes('unexpected token')) {
    return false;
  }
  try {
    if (sessionStorage.getItem('__hangrix_chunk_retry') === '1') {
      return false;
    }
    sessionStorage.setItem('__hangrix_chunk_retry', '1');
  } catch {
    return false;
  }
  location.replace(__hangrixUrlWith({ v: Date.now() }));
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
  __hangrixLoadFailure('module-error', error);
});
</script>`,
  )
  writeFileSync(file, html)
}
