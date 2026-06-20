import tailwindcss from '@tailwindcss/vite'
import legacy from '@vitejs/plugin-legacy'

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
          innerHTML: `;(function(){var logs=[];function add(t,m){logs.push({t:t,m:String(m||''),at:new Date().toISOString()});if(logs.length>20)logs.shift()}window.__hangrixDiagLogs=logs;function text(v){return String(v==null?'':v)}function show(reason){function run(){var root=document.getElementById('__nuxt');var body=document.body;if(!body)return;var existing=document.getElementById('__hangrix_diag');if(existing)return;var pre=document.createElement('pre');pre.id='__hangrix_diag';pre.style.cssText='position:fixed;z-index:2147483647;inset:0;margin:0;padding:14px;overflow:auto;background:#111827;color:#f9fafb;font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:pre-wrap;word-break:break-word;';var scripts=[].map.call(document.scripts,function(s){return s.src||'[inline]'}).join('\\n');var links=[].map.call(document.querySelectorAll('link[href]'),function(s){return s.rel+': '+s.href}).join('\\n');pre.textContent='Hangrix mobile diagnostic\\nreason: '+reason+'\\nurl: '+location.href+'\\nua: '+navigator.userAgent+'\\nbodyTextLength: '+(body.innerText||'').length+'\\nnuxtChildren: '+(root?root.children.length:'missing')+'\\nlogs:\\n'+logs.map(function(x){return '['+x.t+'] '+x.m}).join('\\n')+'\\n\\nscripts:\\n'+scripts+'\\n\\nlinks:\\n'+links;body.appendChild(pre)}if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',run,{once:true});else run()}function criticalResource(tag,rel,ref){if(!ref)return false;if(tag==='LINK'&&rel==='modulepreload')return false;try{var u=new URL(ref,location.href);if(u.origin!==location.origin)return false;return u.pathname.indexOf('/_nuxt/')===0||tag==='SCRIPT'||rel==='stylesheet'}catch(e){return ref.indexOf('/_nuxt/')===0}}function maybeRecover(error){var msg=text(error&&error.stack||error&&error.message||error);var lower=msg.toLowerCase();if(lower.indexOf('dynamically imported module')===-1&&lower.indexOf('importing a module script failed')===-1&&lower.indexOf('failed to fetch')===-1&&lower.indexOf('unexpected token')===-1)return false;if(sessionStorage.getItem('__hangrix_chunk_retry')==='1')return false;sessionStorage.setItem('__hangrix_chunk_retry','1');add('recover','hard-reload');location.replace(location.pathname+location.search+(location.search?'&':'?')+'v='+(Date.now()));return true}window.addEventListener('pageshow',function(){try{sessionStorage.removeItem('__hangrix_chunk_retry')}catch(e){}});window.addEventListener('error',function(e){var target=e.target;if(target&&target!==window){var tag=text(target.tagName);var rel=text(target.rel);var ref=text(target.src||target.href);add('resource',tag+' '+ref);if(!criticalResource(tag,rel,ref)){return}show('resource-error');return}add('error',text(e.message)+' '+text(e.filename)+':'+text(e.lineno)+':'+text(e.colno));show('js-error')},true);window.addEventListener('unhandledrejection',function(e){add('promise',e.reason&&e.reason.stack||e.reason);if(maybeRecover(e.reason)){return}show('promise-error')});setTimeout(function(){var root=document.getElementById('__nuxt');var body=document.body;var txt=body&&body.innerText?body.innerText.trim():'';if(!root||(root.children.length===0&&txt.length===0))show('empty-after-timeout')},10000);})();`,
        },
        {
          innerHTML: `;(function(){var A=Array.prototype,S=String.prototype;function at(i){var o=Object(this),l=o.length>>>0,n=Math.trunc(i)||0,k=n<0?l+n:n;return k<0||k>=l?void 0:o[k]}function findLastIndex(cb,thisArg){if(this==null)throw new TypeError('Array.prototype.findLastIndex called on null or undefined');if(typeof cb!=='function')throw new TypeError('predicate must be a function');for(var o=Object(this),l=o.length>>>0,i=l-1;i>=0;i--){if(cb.call(thisArg,o[i],i,o))return i}return-1}function findLast(cb,thisArg){var i=findLastIndex.call(this,cb,thisArg);return i<0?void 0:this[i]}if(!A.at)Object.defineProperty(A,'at',{value:at,configurable:true,writable:true});if(!S.at)Object.defineProperty(S,'at',{value:at,configurable:true,writable:true});if(!A.findLastIndex)Object.defineProperty(A,'findLastIndex',{value:findLastIndex,configurable:true,writable:true});if(!A.findLast)Object.defineProperty(A,'findLast',{value:findLast,configurable:true,writable:true});if(!A.toReversed)Object.defineProperty(A,'toReversed',{value:function(){return Array.prototype.slice.call(this).reverse()},configurable:true,writable:true});if(!A.toSorted)Object.defineProperty(A,'toSorted',{value:function(c){return Array.prototype.slice.call(this).sort(c)},configurable:true,writable:true});if(!A.toSpliced)Object.defineProperty(A,'toSpliced',{value:function(s,d){var a=Array.prototype.slice.call(this);var r=Array.prototype.slice.call(arguments,2);a.splice.apply(a,[s,d].concat(r));return a},configurable:true,writable:true});if(!A.with)Object.defineProperty(A,'with',{value:function(i,v){var a=Array.prototype.slice.call(this);var n=i<0?a.length+i:i;if(n<0||n>=a.length)throw new RangeError('index out of range');a[n]=v;return a},configurable:true,writable:true});if(!Object.hasOwn)Object.hasOwn=function(o,p){return Object.prototype.hasOwnProperty.call(Object(o),p)}})();`,
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
    componentDir: '@/components/ui'
  },
  typescript: {
    strict: true,
    typeCheck: false
  },
  runtimeConfig: {
    public: {
      apiBase: ''
    }
  },
  nitro: {
    devProxy: {
      '/api': { target: 'http://localhost:8080/api', changeOrigin: true },
      // Forward git smart-HTTP so the clone URL displayed in the UI
      // (which uses window.location.origin = the dev host) actually works.
      // In prod the single binary serves /git itself, no proxy needed.
      '/git': { target: 'http://localhost:8080/git', changeOrigin: true },
      // Runner install endpoints (bash script + binary). Same story as
      // /git — in prod the single binary serves these itself; in dev
      // Nuxt is the public entrypoint, so without this proxy a fresh
      // `curl localhost:3000/install/runner.sh` would hit the SPA
      // fallback and download index.html instead of the script.
      '/install': { target: 'http://localhost:8080/install', changeOrigin: true },
    }
  }
})
