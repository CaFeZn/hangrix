package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
	"github.com/hangrix/hangrix/pkg/ioc"
)

//go:embed all:dist
var distFS embed.FS

const placeholder = `<!doctype html><meta charset="utf-8"><title>hangrix</title>` +
	`<style>body{font-family:system-ui;padding:2rem;max-width:40rem;margin:auto}</style>` +
	`<h1>hangrix</h1>` +
	`<p>Frontend not bundled into this binary. Build it with:</p>` +
	`<pre><code>pnpm --filter web generate &amp;&amp; pnpm --filter hangrix build</code></pre>`

type SPAHandler struct {
	fsys       fs.FS
	fileServer http.Handler
	hasIndex   bool
}

const viewPreferenceCookie = "hangrix_view"

func NewSPAHandler() *SPAHandler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	_, statErr := fs.Stat(sub, "index.html")
	return &SPAHandler{
		fsys:       sub,
		fileServer: http.FileServer(http.FS(sub)),
		hasIndex:   statErr == nil,
	}
}

func (h *SPAHandler) RegisterRoutes(r chi.Router) {
	r.Handle("/*", h)
}

func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.hasIndex {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	applyViewPreference(w, r, path)
	if shouldServeMobile(path, r) {
		serveMobile(w)
		return
	}
	if path != "" {
		if f, err := h.fsys.Open(path); err == nil {
			f.Close()
			if path == "index.html" {
				w.Header().Set("Cache-Control", "no-store")
			}
			h.fileServer.ServeHTTP(w, r)
			return
		}
		if isBuildAssetPath(path) || !wantsHTMLDocument(r) {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	r.URL.Path = "/"
	h.fileServer.ServeHTTP(w, r)
}

func isBuildAssetPath(path string) bool {
	return strings.HasPrefix(path, "_nuxt/")
}

func wantsHTMLDocument(r *http.Request) bool {
	if dest := strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest")); dest == "document" || dest == "iframe" {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

func applyViewPreference(w http.ResponseWriter, r *http.Request, path string) {
	mode := strings.TrimSpace(r.URL.Query().Get("mobile"))
	switch {
	case r.URL.Query().Get("desktop") == "1":
		http.SetCookie(w, &http.Cookie{
			Name:     viewPreferenceCookie,
			Value:    "desktop",
			Path:     "/",
			MaxAge:   30 * 24 * 60 * 60,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
	case mode == "1" || path == "m" || path == "m/":
		http.SetCookie(w, &http.Cookie{
			Name:     viewPreferenceCookie,
			Value:    "mobile",
			Path:     "/",
			MaxAge:   30 * 24 * 60 * 60,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func prefersDesktopView(r *http.Request) bool {
	if r.URL.Query().Get("desktop") == "1" {
		return true
	}
	if r.URL.Query().Get("mobile") == "1" {
		return false
	}
	cookie, err := r.Cookie(viewPreferenceCookie)
	if err != nil {
		return false
	}
	return cookie.Value == "desktop"
}

func shouldServeMobile(path string, r *http.Request) bool {
	if prefersDesktopView(r) {
		return false
	}
	if r.URL.Query().Get("mobile") == "1" || path == "m" || path == "m/" {
		return true
	}
	if !isMobileUA(requestUA(r)) {
		return false
	}
	if !wantsHTMLDocument(r) || isBuildAssetPath(path) {
		return false
	}
	switch strings.Trim(path, "/") {
	case "", "index.html", "projects", "repos", "admin", "admin/dashboard":
		return true
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		switch parts[0] {
		case "api", "git", "install", "admin", "projects", "repos", "login", "register", "profile", "orgs":
			return false
		default:
			return true
		}
	}
	if len(parts) == 3 && parts[0] != "" && parts[1] != "" {
		if parts[2] == "issues" || parts[2] == "settings" {
			return true
		}
	}
	if len(parts) == 4 && parts[0] != "" && parts[1] != "" && parts[2] == "issues" {
		if _, err := strconv.ParseInt(parts[3], 10, 64); err == nil {
			return true
		}
	}
	return false
}

func requestUA(r *http.Request) string {
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		return ua
	}
	return strings.TrimSpace(r.Header.Get("X-Original-User-Agent"))
}

func isMobileUA(ua string) bool {
	ua = strings.ToLower(ua)
	if ua == "" {
		return false
	}
	return strings.Contains(ua, "android") ||
		strings.Contains(ua, "iphone") ||
		strings.Contains(ua, "ipad") ||
		strings.Contains(ua, "mobile") ||
		strings.Contains(ua, "harmonyos")
}

func serveMobile(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(mobileHTML))
}

const mobileHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>Hangrix Mobile</title>
<style>
:root{color-scheme:dark;background:#09090b;color:#fafafa;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
*{box-sizing:border-box}body{margin:0;min-height:100vh;background:#09090b;color:#fafafa}
.wrap{max-width:560px;margin:0 auto;padding:24px 16px 40px}
.top{display:flex;justify-content:space-between;align-items:center;margin-bottom:18px;gap:12px}
.brand{font-size:22px;font-weight:700}
.muted{color:#a1a1aa;font-size:14px;line-height:1.6}
.panel{border:1px solid #27272a;background:#111113;border-radius:14px;padding:18px}
.nav{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin:16px 0 18px}
.nav a{display:flex;align-items:center;justify-content:center;height:38px;border:1px solid #2f2f35;border-radius:10px;text-decoration:none;color:#d4d4d8;background:#121215;font-size:13px}
.nav a.active{border-color:#fafafa;background:#fafafa;color:#09090b;font-weight:650}
.section-title{font-size:20px;font-weight:700;margin:0 0 6px}.section-desc{margin:0 0 14px;color:#a1a1aa;font-size:13px}
.list{display:grid;gap:10px}
.row{display:block;padding:12px;border:1px solid #27272a;border-radius:12px;text-decoration:none;color:#fafafa;background:#151518}
.row-title{display:flex;justify-content:space-between;gap:10px;align-items:flex-start;font-size:15px;font-weight:650}
.row-meta{margin-top:6px;font-size:12px;color:#8f8f98}
.row-desc{margin-top:8px;font-size:13px;color:#d4d4d8;line-height:1.5}
.badge{display:inline-flex;align-items:center;justify-content:center;padding:2px 8px;border:1px solid #3f3f46;border-radius:999px;font-size:11px;color:#d4d4d8}
.stats{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}
.stat{border:1px solid #27272a;border-radius:12px;padding:12px;background:#151518}
.stat-label{font-size:12px;color:#8f8f98}
.stat-value{margin-top:6px;font-size:24px;font-weight:700}
.actions{display:grid;gap:10px;margin-top:14px}
label{display:block;margin:14px 0 6px;color:#d4d4d8;font-size:14px}
input{width:100%;height:44px;border:1px solid #3f3f46;border-radius:10px;background:#09090b;color:#fafafa;padding:0 12px;font-size:16px}
button,.btn{display:inline-flex;align-items:center;justify-content:center;width:100%;height:44px;border:1px solid #fafafa;border-radius:10px;background:#fafafa;color:#09090b;font-weight:650;font-size:15px;text-decoration:none;margin-top:16px}
.ghost{background:transparent;color:#fafafa;border-color:#3f3f46}
.danger{color:#fca5a5}
.small{font-size:12px;color:#71717a;margin-top:14px}
.empty,.hint{font-size:13px;color:#a1a1aa}
.toolbar{display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:12px}
.linkline{color:#d4d4d8;text-decoration:none;font-size:13px}
</style>
</head>
<body>
<main class="wrap">
  <div class="top"><div class="brand">Hangrix</div><a class="muted" href="/?desktop=1">PC版</a></div>
  <section id="app" class="panel">
    <p class="muted">加载中...</p>
  </section>
</main>
<script>
(function(){
  var app=document.getElementById('app');
  var path=(location.pathname||'/').replace(/\/+$/,'')||'/';
  if(path==='/admin')path='/admin/dashboard';
  function esc(s){return String(s||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
  function fmt(n){return Number(n||0).toLocaleString()}
  function desktopHref(p){return p+(p.indexOf('?')>=0?'&':'?')+'desktop=1'}
  function badge(text){return '<span class="badge">'+esc(text)+'</span>'}
  function req(method,url,body,cb){
    var x=new XMLHttpRequest();
    x.open(method,url,true);
    x.setRequestHeader('Accept','application/json');
    if(body)x.setRequestHeader('Content-Type','application/json');
    x.onreadystatechange=function(){
      if(x.readyState!==4)return;
      var data=null;try{data=x.responseText?JSON.parse(x.responseText):null}catch(e){}
      cb(x.status,data);
    };
    x.send(body?JSON.stringify(body):null);
  }
  function nav(active){
    var items=[
      ['/', '首页'],
      ['/projects', '项目'],
      ['/repos', '仓库'],
      ['/admin/dashboard', '后台']
    ];
    return '<nav class="nav">'+items.map(function(it){
      return '<a href="'+it[0]+'" class="'+(active===it[0]?'active':'')+'">'+it[1]+'</a>';
    }).join('')+'</nav>';
  }
  function shell(active,title,desc,body,user){
    var head='<div class="toolbar"><div><h1 class="section-title">'+esc(title)+'</h1><p class="section-desc">'+esc(desc)+'</p></div>'+(user?'<a class="linkline" href="'+desktopHref(path)+'">PC版</a>':'')+'</div>';
    var foot=user?'<div class="actions"><button id="logout" class="ghost">退出登录</button></div><p class="small">当前账号：'+esc(user.username)+'</p>':'<p class="small">手机端轻量页面，适合登录、看项目列表、仓库列表和后台概览。</p>';
    app.innerHTML=head+nav(active)+body+foot;
    var logout=document.getElementById('logout');
    if(logout)logout.onclick=function(){req('POST','/api/auth/logout',null,function(){location.href='/'})};
  }
  function loginView(err){
    app.innerHTML=
      '<h1 class="section-title">登录</h1>'+
      '<p class="section-desc">手机端轻量页面，不加载 PC 版 Nuxt 前端。</p>'+
      (err?'<p class="danger">'+esc(err)+'</p>':'')+
      '<label>用户名</label><input id="u" autocomplete="username">'+
      '<label>密码</label><input id="p" type="password" autocomplete="current-password">'+
      '<button id="login">登录</button>'+
      '<a class="btn ghost" href="/register?desktop=1">注册新账号</a>'+
      '<p class="small">登录后可进入项目、仓库和后台概览。需要完整功能时再切 PC 版。</p>';
    document.getElementById('login').onclick=function(){
      var username=document.getElementById('u').value;
      var password=document.getElementById('p').value;
      req('POST','/api/auth/login',{username:username,password:password},function(status,data){
        if(status>=200&&status<300){location.reload();return}
        loginView(data&&data.error?data.error:'登录失败');
      });
    };
  }
  function homeView(user){
    shell('/', '首页', '移动工作台', ''+
      '<div class="list">'+
      '<a class="row" href="/projects"><div class="row-title"><span>项目</span>'+badge('Projects')+'</div><div class="row-desc">查看你可访问的项目列表。</div></a>'+
      '<a class="row" href="/repos"><div class="row-title"><span>仓库</span>'+badge('Repos')+'</div><div class="row-desc">查看你的仓库列表和基础信息。</div></a>'+
      '<a class="row" href="/admin/dashboard"><div class="row-title"><span>管理后台</span>'+badge('Admin')+'</div><div class="row-desc">查看后台概览、Runner 和调用健康状态。</div></a>'+
      '<a class="row" href="'+desktopHref('/')+'"><div class="row-title"><span>打开完整 PC 版</span></div><div class="row-desc">仅在桌面浏览器中使用完整页面。</div></a>'+
      '</div>', user);
  }
  function projectsView(user){
    shell('/projects','项目','读取项目列表中...','<p class="hint">正在加载项目...</p>',user);
    req('GET','/api/projects',null,function(status,data){
      if(status===401){loginView();return}
      if(status<200||status>=300){shell('/projects','项目','项目列表加载失败','<p class="danger">'+esc(data&&data.error||'加载失败')+'</p>',user);return}
      var items=(data&&data.items)||[];
      var body=items.length?'<div class="list">'+items.map(function(it){
        var owner=it.owner_name||'';
        var name=it.name||'';
        var desc=it.description||'暂无描述';
        return '<div class="row"><div class="row-title"><span>'+esc(owner)+' / '+esc(name)+'</span>'+badge(it.visibility||'private')+'</div><div class="row-desc">'+esc(desc)+'</div><div class="row-meta">更新时间：'+esc((it.updated_at||'').replace('T',' ').replace('Z',''))+'</div></div>';
      }).join('')+'</div>':'<p class="empty">没有可访问的项目。</p>';
      shell('/projects','项目','共 '+fmt(data&&data.total)+' 个项目',body,user);
    });
  }
  function reposView(user){
    shell('/repos','仓库','读取仓库列表中...','<p class="hint">正在加载仓库...</p>',user);
    req('GET','/api/repos/me',null,function(status,data){
      if(status===401){loginView();return}
      if(status<200||status>=300){shell('/repos','仓库','仓库列表加载失败','<p class="danger">'+esc(data&&data.error||'加载失败')+'</p>',user);return}
      var items=(data&&data.items)||[];
      var body=items.length?'<div class="list">'+items.map(function(it){
        var owner=it.owner_name||it.owner_username||'';
        var name=it.name||'';
        var desc=it.description||'暂无描述';
        var branch=it.default_branch||'main';
        return '<a class="row" href="/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name)+'"><div class="row-title"><span>'+esc(owner)+' / '+esc(name)+'</span>'+badge(it.visibility||'private')+'</div><div class="row-desc">'+esc(desc)+'</div><div class="row-meta">默认分支：'+esc(branch)+' · 权限：'+esc(it.viewer_permission||'read')+'</div></a>';
      }).join('')+'</div>':'<p class="empty">没有仓库。</p>';
      shell('/repos','仓库','共 '+fmt(data&&data.total)+' 个仓库',body,user);
    });
  }
  function repoDetailView(user,owner,name){
    shell('/repos','仓库详情','读取仓库详情中...','<p class="hint">正在加载仓库详情...</p>',user);
    req('GET','/api/repos/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name),null,function(status,data){
      if(status===401){loginView();return}
      if(status===404){shell('/repos','仓库详情','仓库不存在','<p class="danger">没有找到这个仓库。</p>',user);return}
      if(status<200||status>=300){shell('/repos','仓库详情','仓库详情加载失败','<p class="danger">'+esc(data&&data.error||'加载失败')+'</p>',user);return}
      var desc=data&&data.description||'暂无描述';
      var visibility=data&&data.visibility||'private';
      var branch=data&&data.default_branch||'main';
      var perm=data&&data.viewer_permission||'read';
      var base='/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name);
      var silence=(data&&data.silence&&data.silence.active)?'<div class="row"><div class="row-title"><span>静默状态</span>'+badge(data.silence.source||'silenced')+'</div><div class="row-desc">该仓库当前处于静默状态。</div></div>':'';
      var body=
        '<div class="list">'+
        '<div class="row"><div class="row-title"><span>'+esc(owner)+' / '+esc(name)+'</span>'+badge(visibility)+'</div><div class="row-desc">'+esc(desc)+'</div><div class="row-meta">默认分支：'+esc(branch)+' · 权限：'+esc(perm)+'</div></div>'+
        silence+
        '<a class="row" href="'+desktopHref(base)+'"><div class="row-title"><span>打开完整仓库页</span></div><div class="row-desc">桌面版可查看代码、提交、文件树和设置。</div></a>'+
        '<a class="row" href="'+base+'/issues"><div class="row-title"><span>查看 Issues</span></div><div class="row-desc">进入移动版 Issue 列表。</div></a>'+
        '<a class="row" href="'+base+'/settings"><div class="row-title"><span>仓库设置</span></div><div class="row-desc">进入移动版设置概览。</div></a>'+
        '</div>';
      shell('/repos','仓库详情',owner+' / '+name,body,user);
    });
  }
  function issuesView(user,owner,name){
    shell('/repos','Issues','读取 Issue 列表中...','<p class="hint">正在加载 Issues...</p>',user);
    req('GET','/api/repos/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name)+'/issues',null,function(status,data){
      if(status===401){loginView();return}
      if(status<200||status>=300){shell('/repos','Issues','Issue 列表加载失败','<p class="danger">'+esc(data&&data.error||'加载失败')+'</p>',user);return}
      var items=(data&&data.items)||[];
      var body=items.length?'<div class="list">'+items.map(function(it){
        var state=it.state||'open';
        var title=it.title||('Issue #'+it.number);
        var summary=(it.body||'').replace(/\s+/g,' ').slice(0,120)||'暂无描述';
        return '<a class="row" href="/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name)+'/issues/'+it.number+'"><div class="row-title"><span>#'+it.number+' '+esc(title)+'</span>'+badge(state)+'</div><div class="row-desc">'+esc(summary)+'</div><div class="row-meta">分支：'+esc(it.branch_name||'')+'</div></a>';
      }).join('')+'</div>':'<p class="empty">这个仓库还没有 Issue。</p>';
      shell('/repos','Issues',owner+' / '+name+' · 共 '+fmt(data&&data.total)+' 条',body,user);
    });
  }
  function issueDetailView(user,owner,name,number){
    shell('/repos','Issue 详情','读取 Issue 详情中...','<p class="hint">正在加载 Issue 详情...</p>',user);
    req('GET','/api/repos/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name)+'/issues/'+encodeURIComponent(number),null,function(status,data){
      if(status===401){loginView();return}
      if(status===404){shell('/repos','Issue 详情','Issue 不存在','<p class="danger">没有找到这个 Issue。</p>',user);return}
      if(status<200||status>=300){shell('/repos','Issue 详情','Issue 详情加载失败','<p class="danger">'+esc(data&&data.error||'加载失败')+'</p>',user);return}
      var bodyText=data&&data.body||'暂无描述';
      var todos=data&&data.todo_summary?'<div class="row"><div class="row-title"><span>Todo</span>'+badge((data.todo_summary.done||0)+' / '+(data.todo_summary.total||0))+'</div><div class="row-meta">待办 '+fmt(data.todo_summary.todo)+' · 进行中 '+fmt(data.todo_summary.in_progress)+' · 完成 '+fmt(data.todo_summary.done)+'</div></div>':'';
      var body=
        '<div class="list">'+
        '<div class="row"><div class="row-title"><span>#'+esc(data.number)+' '+esc(data.title||'')+'</span>'+badge(data.state||'open')+'</div><div class="row-desc">'+esc(bodyText)+'</div><div class="row-meta">分支：'+esc(data.branch_name||'')+' · 基线：'+esc(data.base_branch||'')+'</div></div>'+
        todos+
        '<a class="row" href="/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name)+'/issues"><div class="row-title"><span>返回 Issue 列表</span></div><div class="row-desc">回到这个仓库的移动版 Issue 列表。</div></a>'+
        '<a class="row" href="'+desktopHref('/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name)+'/issues/'+encodeURIComponent(number))+'"><div class="row-title"><span>打开完整 Issue 页</span></div><div class="row-desc">桌面版可查看时间线、评论、贡献和 Agent Session。</div></a>'+
        '</div>';
      shell('/repos','Issue 详情',owner+' / '+name+' · #'+number,body,user);
    });
  }
  function settingsView(user,owner,name){
    shell('/repos','仓库设置','读取设置概览中...','<p class="hint">正在加载仓库设置...</p>',user);
    var repoUrl='/api/repos/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name);
    req('GET',repoUrl,null,function(status,repo){
      if(status===401){loginView();return}
      if(status<200||status>=300){shell('/repos','仓库设置','设置概览加载失败','<p class="danger">'+esc(repo&&repo.error||'加载失败')+'</p>',user);return}
      req('GET',repoUrl+'/members',null,function(_mStatus,members){
        req('GET',repoUrl+'/variables',null,function(_vStatus,vars){
          var memberCount=(members&&members.items&&members.items.length)||0;
          var plainVars=(vars&&vars.variables&&vars.variables.length)||0;
          var secrets=(vars&&vars.secrets&&vars.secrets.length)||0;
          var body=
            '<div class="stats">'+
            '<div class="stat"><div class="stat-label">可见性</div><div class="stat-value" style="font-size:18px">'+esc(repo.visibility||'private')+'</div></div>'+
            '<div class="stat"><div class="stat-label">默认分支</div><div class="stat-value" style="font-size:18px">'+esc(repo.default_branch||'main')+'</div></div>'+
            '<div class="stat"><div class="stat-label">成员数</div><div class="stat-value">'+fmt(memberCount)+'</div></div>'+
            '<div class="stat"><div class="stat-label">变量 / 密钥</div><div class="stat-value">'+fmt(plainVars)+' / '+fmt(secrets)+'</div></div>'+
            '</div>'+
            '<div class="list" style="margin-top:12px">'+
            '<div class="row"><div class="row-title"><span>仓库描述</span></div><div class="row-desc">'+esc(repo.description||'暂无描述')+'</div></div>'+
            '<a class="row" href="'+desktopHref('/'+encodeURIComponent(owner)+'/'+encodeURIComponent(name)+'/settings')+'"><div class="row-title"><span>打开完整设置页</span></div><div class="row-desc">桌面版可管理成员、变量、Runner、.hangrix、分支保护等。</div></a>'+
            '</div>';
          shell('/repos','仓库设置',owner+' / '+name,body,user);
        });
      });
    });
  }
  function adminView(user){
    shell('/admin/dashboard','管理后台','读取后台概览中...','<p class="hint">正在加载后台概览...</p>',user);
    req('GET','/api/admin/dashboard',null,function(status,data){
      if(status===401){loginView();return}
      if(status===403){shell('/admin/dashboard','管理后台','无权限','<p class="danger">当前账号没有管理员权限。</p>',user);return}
      if(status<200||status>=300){shell('/admin/dashboard','管理后台','后台概览加载失败','<p class="danger">'+esc(data&&data.error||'加载失败')+'</p>',user);return}
      var s=data&&data.summary||{}, h=data&&data.health||{}, fails=data&&data.recent_failures||[];
      var body=
        '<div class="stats">'+
        '<div class="stat"><div class="stat-label">总调用</div><div class="stat-value">'+fmt(s.total_calls)+'</div></div>'+
        '<div class="stat"><div class="stat-label">总 Tokens</div><div class="stat-value">'+fmt(s.total_tokens)+'</div></div>'+
        '<div class="stat"><div class="stat-label">活跃 Session</div><div class="stat-value">'+fmt(s.active_sessions)+'</div></div>'+
        '<div class="stat"><div class="stat-label">在线 Runner</div><div class="stat-value">'+fmt(s.online_runners)+' / '+fmt(s.total_runners)+'</div></div>'+
        '</div>'+
        '<div class="list" style="margin-top:12px">'+
        '<div class="row"><div class="row-title"><span>Runner 健康</span></div><div class="row-meta">在线 '+fmt(h.online_runners)+' · 离线 '+fmt(h.offline_runners)+' · 禁用 '+fmt(h.disabled_runners)+' · 活跃 '+fmt(h.live_sessions)+'</div></div>'+
        (fails.length?fails.slice(0,5).map(function(it){
          return '<div class="row"><div class="row-title"><span>'+esc(it.provider_name||'provider')+' / '+esc(it.model||'model')+'</span>'+badge(String(it.status_code||0))+'</div><div class="row-desc">'+esc(it.error_message||'调用失败')+'</div></div>';
        }).join(''):'<div class="row"><div class="row-title"><span>最近失败</span></div><div class="row-desc">最近没有失败记录。</div></div>')+
        '</div>';
      shell('/admin/dashboard','管理后台','管理员概览',body,user);
    });
  }
  function route(user){
    if(path==='/projects')return projectsView(user);
    if(path==='/repos')return reposView(user);
    if(path==='/admin/dashboard')return adminView(user);
    var parts=path.replace(/^\/+/,'').split('/');
    if(parts.length===3&&parts[0]&&parts[1]&&parts[2]==='issues')return issuesView(user,parts[0],parts[1]);
    if(parts.length===4&&parts[0]&&parts[1]&&parts[2]==='issues')return issueDetailView(user,parts[0],parts[1],parts[3]);
    if(parts.length===3&&parts[0]&&parts[1]&&parts[2]==='settings')return settingsView(user,parts[0],parts[1]);
    if(parts.length===2&&parts[0]&&parts[1])return repoDetailView(user,parts[0],parts[1]);
    return homeView(user);
  }
  req('GET','/api/auth/me',null,function(status,data){
    if(status===401){loginView();return}
    if(status<200||status>=300){loginView('会话校验失败');return}
    route(data||{});
  });
})();
</script>
</body>
</html>`

func Module() *ioc.Module {
	m := ioc.NewModule()
	m.Provide(NewSPAHandler).ToInterface(new(server.RouteProvider))
	return m
}
