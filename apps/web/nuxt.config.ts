import tailwindcss from '@tailwindcss/vite'
import legacy from '@vitejs/plugin-legacy'

const startupRecoveryScript = `;(function(){var logs=[];function text(v){return String(v==null?'':v)}function add(t,m){var item={t:t,m:text(m),at:new Date().toISOString()};logs.push(item);if(logs.length>20)logs.shift();try{console.warn('[hangrix-startup]',t,item.m)}catch(e){}}window.__hangrixDiagLogs=logs;function escaped(s){return text(s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}function urlWith(params){var u=new URL(location.href);Object.keys(params).forEach(function(k){if(params[k]==null)u.searchParams.delete(k);else u.searchParams.set(k,params[k])});return u.toString()}function hasAppContent(){var root=document.getElementById('__nuxt');var body=document.body;var bodyText=body&&body.innerText?body.innerText.trim():'';return !!(root&&root.children.length>0)||bodyText.length>0}function isChunkError(error){var lower=text(error&&error.stack||error&&error.message||error).toLowerCase();return lower.indexOf('dynamically imported module')!==-1||lower.indexOf('importing a module script failed')!==-1||lower.indexOf('failed to fetch')!==-1||lower.indexOf('unexpected token')!==-1}function maybeRecover(error){if(!isChunkError(error))return false;try{if(sessionStorage.getItem('__hangrix_chunk_retry')==='1')return false;sessionStorage.setItem('__hangrix_chunk_retry','1')}catch(e){return false}add('recover','hard-reload');location.replace(urlWith({v:Date.now()}));return true}function shouldShowDebug(){try{return new URLSearchParams(location.search).has('hangrix_diag')||localStorage.getItem('__hangrix_diag')==='1'}catch(e){return false}}function showLoadFailure(reason,error){if(hasAppContent()||document.getElementById('__hangrix_load_failure'))return;var body=document.body||document.documentElement;if(!body)return;var details=text(error&&error.stack||error&&error.message||error);var panel=document.createElement('div');panel.id='__hangrix_load_failure';panel.style.cssText='position:fixed;z-index:2147483647;inset:0;display:flex;align-items:center;justify-content:center;padding:20px;background:#09090b;color:#fafafa;font:14px/1.5 system-ui,-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;';var card=document.createElement('div');card.style.cssText='width:min(520px,100%);border:1px solid #27272a;border-radius:12px;background:#111113;padding:20px;box-shadow:0 20px 60px rgba(0,0,0,.35)';var debug=shouldShowDebug()?'<pre style="margin:14px 0 0;max-height:180px;overflow:auto;border:1px solid #27272a;border-radius:8px;padding:10px;background:#09090b;color:#d4d4d8;font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:pre-wrap;word-break:break-word;">reason: '+escaped(reason)+'\\nurl: '+escaped(location.href)+'\\nua: '+escaped(navigator.userAgent)+'\\nerror: '+escaped(details)+'\\nlogs:\\n'+escaped(logs.map(function(x){return '['+x.t+'] '+x.m}).join('\\n'))+'</pre>':'';card.innerHTML='<h1 style="margin:0 0 8px;font-size:20px;line-height:1.25">Hangrix 页面加载失败</h1><p style="margin:0 0 16px;color:#a1a1aa">页面资源可能还在更新或网络请求被中断。先刷新一次；如果仍失败，可以临时打开轻量版。</p><div style="display:grid;gap:10px;grid-template-columns:repeat(auto-fit,minmax(120px,1fr))"><button id="__hangrix_retry" style="height:40px;border:1px solid #fafafa;border-radius:8px;background:#fafafa;color:#09090b;font-weight:650;cursor:pointer">重新加载</button><a style="height:40px;border:1px solid #3f3f46;border-radius:8px;color:#fafafa;text-decoration:none;display:flex;align-items:center;justify-content:center" href="'+escaped(urlWith({desktop:'1',mobile:null,v:Date.now()}))+'">强制桌面版</a><a style="height:40px;border:1px solid #3f3f46;border-radius:8px;color:#fafafa;text-decoration:none;display:flex;align-items:center;justify-content:center" href="'+escaped(urlWith({mobile:'1',desktop:null,v:Date.now()}))+'">打开轻量版</a></div>'+debug;panel.appendChild(card);body.appendChild(panel);var retry=document.getElementById('__hangrix_retry');if(retry)retry.onclick=function(){location.replace(urlWith({v:Date.now()}))}}window.addEventListener('pageshow',function(){try{sessionStorage.removeItem('__hangrix_chunk_retry')}catch(e){}});window.addEventListener('error',function(e){var target=e.target;if(target&&target!==window){add('resource',text(target.tagName)+' '+text(target.src||target.href));return}add('error',text(e.message)+' '+text(e.filename)+':'+text(e.lineno)+':'+text(e.colno));if(!maybeRecover(e.error||e.message))showLoadFailure('js-error',e.error||e.message)},true);window.addEventListener('unhandledrejection',function(e){add('promise',e.reason&&e.reason.stack||e.reason);if(!maybeRecover(e.reason))showLoadFailure('promise-error',e.reason)});setTimeout(function(){if(!hasAppContent())add('empty-after-10s','nuxt root still empty')},10000);setTimeout(function(){if(!hasAppContent())showLoadFailure('empty-after-timeout')},30000);})();`

const legacyPolyfillScript = `;(function(){var A=Array.prototype,S=String.prototype;function at(i){var o=Object(this),l=o.length>>>0,n=Math.trunc(i)||0,k=n<0?l+n:n;return k<0||k>=l?void 0:o[k]}function findLastIndex(cb,thisArg){if(this==null)throw new TypeError('Array.prototype.findLastIndex called on null or undefined');if(typeof cb!=='function')throw new TypeError('predicate must be a function');for(var o=Object(this),l=o.length>>>0,i=l-1;i>=0;i--){if(cb.call(thisArg,o[i],i,o))return i}return-1}function findLast(cb,thisArg){var i=findLastIndex.call(this,cb,thisArg);return i<0?void 0:this[i]}if(!A.at)Object.defineProperty(A,'at',{value:at,configurable:true,writable:true});if(!S.at)Object.defineProperty(S,'at',{value:at,configurable:true,writable:true});if(!A.findLastIndex)Object.defineProperty(A,'findLastIndex',{value:findLastIndex,configurable:true,writable:true});if(!A.findLast)Object.defineProperty(A,'findLast',{value:findLast,configurable:true,writable:true});if(!A.toReversed)Object.defineProperty(A,'toReversed',{value:function(){return Array.prototype.slice.call(this).reverse()},configurable:true,writable:true});if(!A.toSorted)Object.defineProperty(A,'toSorted',{value:function(c){return Array.prototype.slice.call(this).sort(c)},configurable:true,writable:true});if(!A.toSpliced)Object.defineProperty(A,'toSpliced',{value:function(s,d){var a=Array.prototype.slice.call(this);var r=Array.prototype.slice.call(arguments,2);a.splice.apply(a,[s,d].concat(r));return a},configurable:true,writable:true});if(!A.with)Object.defineProperty(A,'with',{value:function(i,v){var a=Array.prototype.slice.call(this);var n=i<0?a.length+i:i;if(n<0||n>=a.length)throw new RangeError('index out of range');a[n]=v;return a},configurable:true,writable:true});if(!Object.hasOwn)Object.hasOwn=function(o,p){return Object.prototype.hasOwnProperty.call(Object(o),p)}})();`

export default defineNuxtConfig({
  compatibilityDate: '2026-05-13',
  devtools: { enabled: true },
  modules: ['shadcn-nuxt', '@nuxtjs/i18n'],
  i18n: {
    strategy: 'no_prefix',
    defaultLocale: 'zh-CN',
    locales: [
      { code: 'zh-CN', name: '简体中文', file: 'zh-CN.json' },
      { code: 'en', name: 'English', file: 'en.json' },
    ],
    detectBrowserLanguage: {
      useCookie: true,
      cookieKey: 'hangrix_locale',
      redirectOn: 'root',
      alwaysRedirect: false,
      fallbackLocale: 'zh-CN',
    },
  },
  css: ['~/assets/css/tailwind.css'],
  ssr: false,
  // nuxt issue: https://github.com/nuxt/nuxt/issues/35033
  experimental: {
    viteEnvironmentApi: true,
  },
  app: {
    head: {
      htmlAttrs: { class: 'dark' },
      script: [
        {
          innerHTML: startupRecoveryScript,
        },
        {
          innerHTML: legacyPolyfillScript,
        },
      ],
    },
  },
  vite: {
    plugins: [
      tailwindcss(),
      legacy({
        renderLegacyChunks: false,
        modernPolyfills: [
          'es.array.find-last',
          'es.array.find-last-index',
          'es.array.to-reversed',
          'es.array.to-sorted',
          'es.array.to-spliced',
          'es.array.with',
          'es.object.has-own',
        ],
      }),
    ],
    esbuild: {
      target: 'es2019',
    },
    build: {
      target: 'es2019',
      cssTarget: 'chrome61',
      modulePreload: false,
    },
  },
  shadcn: {
    prefix: '',
    componentDir: '@/components/ui',
  },
  typescript: {
    strict: true,
    typeCheck: false,
  },
  runtimeConfig: {
    public: {
      apiBase: '',
    },
  },
  nitro: {
    devProxy: {
      '/api': { target: 'http://localhost:8080/api', changeOrigin: true },
      // Forward git smart-HTTP so the clone URL displayed in the UI
      // (which uses window.location.origin = the dev host) actually works.
      // In prod the single binary serves /git itself, no proxy needed.
      '/git': { target: 'http://localhost:8080/git', changeOrigin: true },
      // Runner install endpoints (bash script + binary). Same story as
      // /git - in prod the single binary serves these itself; in dev
      // Nuxt is the public entrypoint, so without this proxy a fresh
      // `curl localhost:3000/install/runner.sh` would hit the SPA
      // fallback and download index.html instead of the script.
      '/install': { target: 'http://localhost:8080/install', changeOrigin: true },
    },
  },
})
