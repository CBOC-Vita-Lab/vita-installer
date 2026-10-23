/* CBOC Vita Installer page. Plain ES2017 so it runs in older Safari and Edge. */
(function () {
  'use strict';
  if (!window.fetch || !window.EventSource || !window.Promise) {
    document.body.innerHTML = '<main><h1>Please use a newer browser</h1><p class="muted">Copy this address into Chrome, Firefox, Edge or Safari:</p><p><b>' +
      location.href.replace(/[<>&"]/g, '') + '</b></p></main>';
    return;
  }
  var base = location.pathname.replace(/[^/]*$/, '');
  var S = null;
  var $ = function (id) { return document.getElementById(id); };

  function api(name, body) {
    return fetch(base + 'api/' + name, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'cboc' },
      body: JSON.stringify(body || {})
    }).then(function (r) {
      return r.json().catch(function () { return {}; });
    }).then(function (j) {
      if (j && j.error) showNotice(j.error, true);
      return j;
    }).catch(function () { showNotice('The installer stopped. Open it again to continue.', true); });
  }

  var localNotice = null;
  function showNotice(text, bad) { localNotice = { text: text, bad: bad }; render(); }

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function size(n) {
    if (!n || n < 0) return '0 MB';
    if (n >= 1073741824) return (n / 1073741824).toFixed(n >= 10737418240 ? 0 : 1) + ' GB';
    if (n >= 1048576) return Math.round(n / 1048576) + ' MB';
    return Math.max(1, Math.round(n / 1024)) + ' KB';
  }
  function dur(s) {
    if (s == null || s < 0) return '';
    s = Math.round(s);
    if (s < 60) return s + ' s';
    if (s < 3600) return Math.floor(s / 60) + ' min ' + (s % 60 ? (s % 60) + ' s' : '');
    return Math.floor(s / 3600) + ' h ' + Math.round((s % 3600) / 60) + ' min';
  }

  // ---- live state ----
  function connect() {
    var es = new EventSource(base + 'api/events');
    es.onmessage = function (e) { S = JSON.parse(e.data); render(); };
    es.onerror = function () {
      es.close();
      setTimeout(connect, 1500);
    };
  }

  // ---- rendering ----
  function connection() {
    var pill = $('conn'), text = $('connText');
    pill.className = 'pill';
    if (S.vita) {
      pill.className = 'pill on';
      text.textContent = 'Vita on Wi-Fi · ' + S.vita.host + (S.vita.companion ? ' · Companion' : '');
    } else if (S.drive) {
      pill.className = 'pill on';
      var d = (S.drives || []).filter(function (x) { return x.path === S.drive; })[0];
      text.textContent = 'Vita drive · ' + (d ? d.name : S.drive) + (d && d.free > 0 ? ' · ' + size(d.free) + ' free' : '');
    } else {
      pill.className = 'pill search';
      text.textContent = S.scanned ? 'No Vita found · How to connect' : 'Looking for your Vita…';
    }
    var list = '';
    (S.vitas || []).forEach(function (v) {
      var sel = S.vita && S.vita.host === v.host;
      list += '<button class="choice' + (sel ? ' sel' : '') + '" data-host="' + esc(v.host) + '"><i class="dot"></i><span>Vita at ' + esc(v.host) +
        '</span><em>' + (v.companion ? 'Installs and launches automatically' : 'VitaShell FTP') + '</em></button>';
    });
    (S.drives || []).forEach(function (d) {
      var sel = S.drive === d.path;
      list += '<button class="choice' + (sel ? ' sel' : '') + '" data-drive="' + esc(d.path) + '"><i class="dot"></i><span>' + esc(d.name) +
        '</span><em>Vita memory card' + (d.free > 0 ? ' · ' + size(d.free) + ' free' : '') + '</em></button>';
    });
    if (!list) list = '<p class="muted">' + (S.scanning ? 'Searching your network…' : 'No Vita found yet. Follow one of the steps below; it is found automatically.') + '</p>';
    $('connList').innerHTML = list;
    $('versionText').textContent = 'Version ' + S.version;
  }

  function banners() {
    var u = $('update');
    if (S.update) {
      u.hidden = false;
      u.className = 'banner good';
      u.innerHTML = '<span>Version ' + esc(S.update.version) + ' of the installer is available.</span><button class="btn small" data-open="' + esc(S.update.url) + '">Download</button>';
    } else u.hidden = true;
    var n = $('notice');
    var note = localNotice || (S.notice ? { text: S.notice, bad: S.noticeBad } : null);
    if (note) {
      n.hidden = false;
      n.className = 'banner' + (note.bad ? ' bad' : '');
      n.innerHTML = '<span>' + esc(note.text) + '</span><button class="x" data-dismiss aria-label="Dismiss">×</button>';
    } else n.hidden = true;
  }

  function catalog() {
    var have = {};
    (S.items || []).forEach(function (it) { have[it.name] = true; });
    var h = '';
    (S.catalog || []).forEach(function (c) {
      if (have[c.name]) return;
      h += '<div class="tile"><div class="t"><div>' + esc(c.name) + '</div><span>' + esc(c.about) + '</span>' +
        (c.needs ? '<span>Needs: ' + esc(c.needs) + '</span>' : '') + '</div><button class="btn small primary" data-get="' + esc(c.repo) + '">Get</button></div>';
    });
    $('catalog').innerHTML = h ? '<div class="muted small" style="width:100%;margin-bottom:4px">CBOC releases</div>' + h : '';
  }

  var typeHelp = {
    'wii-disc': 'ISO, RVZ, WBFS or a folder extracted with Dolphin',
    'gc-disc': 'ISO, RVZ or a folder extracted with Dolphin',
    'psp-iso': 'An .iso file of your UMD',
    'shader-compiler': 'Needed once per Vita. Choose libshacccg.suprx',
    'files': 'Choose a file or a folder of them'
  };

  function itemBytes(it) {
    var n = ((it.app && it.app.size) || 0) + (it.bundledBytes || 0);
    (it.inputs || []).forEach(function (x) { if (x.state === 'ok') n += x.bytes || 0; });
    return n;
  }

  function itemCard(it) {
    var app = it.app || {};
    var cls = 'card' + (it.status === 'done' ? ' done' : '') + (it.status === 'failed' ? ' failed' : '');
    var badge = it.status === 'done' && it.result && it.result.installed ? '<span class="badge ok">Installed</span>' :
      it.installed ? '<span class="badge upd">Update</span>' : (S.vita ? '<span class="badge new">New</span>' : '');
    var meta = [esc(it.titleId)];
    if (it.version) meta.push('v' + esc(it.version));
    meta.push(size(itemBytes(it)));
    var icon = app.titleId ? '<img class="icon" src="' + base + 'api/icon?id=' + esc(it.id) + '" alt="">' : '<div class="icon ph">VPK</div>';
    var busy = it.status === 'working' || it.status === 'queued';
    var h = '<article class="' + cls + '" data-item="' + esc(it.id) + '"><div class="card-head">' + icon +
      '<div class="card-title"><h2>' + esc(it.name) + badge + '</h2><div class="meta">' + meta.join(' · ') + '</div></div>' +
      (busy ? '' : '<button class="x" data-remove="' + esc(it.id) + '" aria-label="Remove ' + esc(it.name) + '">×</button>') + '</div>';
    var inputs = it.inputs || [];
    if (inputs.length) {
      h += '<div class="needs">';
      inputs.forEach(function (n) {
        var state = n.state === 'skip' || n.state === 'ok' ? 'ok' : n.state === 'error' ? 'error' : '';
        var mark = state === 'ok' ? '✓' : state === 'error' ? '!' : '';
        var sub = n.message || (n.path ? n.path.split(/[\\/]/).pop() : (n.help || typeHelp[n.type] || (n.optional ? 'Optional' : '')));
        if (n.optional && !n.path && n.state !== 'skip' && n.type !== 'shader-compiler') sub = 'Optional · ' + sub;
        var folderOk = n.type === 'wii-disc' || n.type === 'gc-disc' || n.type === 'folder' || n.type === 'files';
        h += '<div class="need ' + state + '" data-drop-item="' + esc(it.id) + '" data-drop-input="' + esc(n.id) + '"><div class="st">' + mark + '</div>' +
          '<div class="nt"><div>' + esc(n.label) + '</div><span title="' + esc(n.path || '') + '">' + esc(sub) + '</span></div>';
        if (!busy && n.state !== 'skip') {
          if (n.type !== 'folder') h += '<button class="btn small" data-pick-item="' + esc(it.id) + '" data-pick-input="' + esc(n.id) + '" data-kind="file">' + (n.path ? 'Change' : 'Choose') + '</button>';
          if (folderOk) h += '<button class="btn small ghost" data-pick-item="' + esc(it.id) + '" data-pick-input="' + esc(n.id) + '" data-kind="folder">Folder</button>';
        }
        h += '</div>';
      });
      h += '</div>';
    }
    if (it.held) h += '<div class="note bad">' + esc(it.held) + '</div>';
    if (it.notes && it.status !== 'done') h += '<div class="note">' + esc(it.notes) + '</div>';
    if (it.status === 'failed' && it.error) h += '<div class="note bad">' + esc(it.error) + '</div>';
    if (it.status === 'done' && it.result) {
      var r = it.result;
      var line = r.installed ? 'Installed on your Vita' : 'Copied to your Vita';
      var stats = r.bytes > 0 ? size(r.bytes) + ' in ' + dur(r.seconds) : 'Nothing new to send';
      if (r.skipped) stats += ' · ' + r.skipped + ' file' + (r.skipped === 1 ? '' : 's') + ' already there';
      h += '<div class="note good result"><div><b>' + line + '.</b> <span class="muted">' + stats + '</span>';
      if (r.steps && r.steps.length) {
        h += '<ol>' + r.steps.map(function (s) { return '<li>' + esc(s) + '</li>'; }).join('') + '</ol>';
      }
      h += '</div>';
      if (r.canLaunch && S.vita && S.vita.companion) h += '<button class="btn go small" data-launch="' + esc(it.id) + '">' + (it.launched ? 'Launch again' : 'Play on Vita') + '</button>';
      h += '</div>';
    }
    return h + '</article>';
  }

  function dock() {
    var items = S.items || [];
    var d = $('dock');
    if (!items.length) { d.hidden = true; return; }
    d.hidden = false;
    var ready = items.filter(function (i) { return i.status === 'ready' || i.status === 'failed'; });
    var needs = items.filter(function (i) { return i.status === 'needs'; });
    var connected = !!(S.vita || S.drive);
    var btn = $('installBtn');
    var text = '';
    if (!connected) text = S.scanning ? 'Looking for your Vita…' : 'Connect your Vita to install.';
    else if (!ready.length && needs.length) text = 'Add the missing file' + (needs.length > 1 ? 's' : '') + ' above.';
    else if (!ready.length) text = items.every(function (i) { return i.status === 'done'; }) ? 'All done. Drop another game to keep going.' : '';
    else {
      var total = 0;
      ready.forEach(function (i) { total += itemBytes(i); });
      text = ready.length + ' ready · ' + size(total) + (needs.length ? ' · ' + needs.length + ' waiting for files' : '');
      if (S.vita && total > 700 * 1048576) text += ' · Tip: USB mode is several times faster for big games';
    }
    $('dockText').textContent = text;
    btn.disabled = !connected || !ready.length || !!S.job;
    btn.textContent = ready.length > 1 ? 'Install ' + ready.length + ' games' : (ready.length && ready[0].installed ? 'Update' : 'Install');
  }

  function job() {
    var j = S.job, sheet = $('job');
    if (!j) { sheet.hidden = true; return; }
    sheet.hidden = false;
    var p = j.progress || {};
    var it = (S.items || []).filter(function (i) { return i.id === j.itemId; })[0];
    $('jobTitle').textContent = j.name || 'Preparing';
    $('jobStep').textContent = j.count > 1 ? 'Game ' + (j.index + 1) + ' of ' + j.count : (it && it.installed ? 'Updating' : 'Installing');
    var icon = $('jobIcon');
    var src = it && it.app && it.app.titleId ? base + 'api/icon?id=' + it.id : '';
    if (icon.getAttribute('data-src') !== src) {
      icon.setAttribute('data-src', src);
      icon.style.visibility = src ? '' : 'hidden';
      if (src) icon.src = src;
    }
    var pct = p.total > 0 ? Math.min(100, Math.floor(p.done * 100 / p.total)) : -1;
    var meter = $('jobBar').parentNode;
    if (pct < 0 || p.phase === 'install' || p.phase === 'check') {
      meter.className = 'meter indet';
      $('jobPct').textContent = p.phase === 'install' ? '' : '';
    } else {
      meter.className = 'meter';
      $('jobBar').style.width = pct + '%';
      $('jobPct').textContent = pct + '%';
    }
    $('jobPhase').textContent = p.text || '';
    $('jobSpeed').textContent = p.speed > 0 ? size(p.speed) + '/s' : '';
    $('jobEta').textContent = p.eta > 0 ? dur(p.eta) + ' left' : '';
    var f = '';
    if (p.phase === 'send') f = p.files + ' of ' + p.filesTotal + ' files · ' + size(p.done) + ' of ' + size(p.total) + (p.current ? ' · ' + p.current : '');
    else if (p.done > 0) f = size(p.done) + (p.total > 0 ? ' of ' + size(p.total) : '');
    $('jobFile').textContent = f;
  }

  function task() {
    var t = S.task;
    if (!t && clientTask) { taskEl.hidden = false; taskEl.innerHTML = '<span>' + esc(clientTask) + '</span>'; return; }
    if (!t) { taskEl.hidden = true; return; }
    taskEl.hidden = false;
    var pct = t.total > 0 ? Math.floor(t.done * 100 / t.total) : -1;
    taskEl.innerHTML = '<span>' + esc(t.text) + (pct >= 0 ? ' · ' + pct + '%' : '…') + '</span>';
  }
  var taskEl = document.createElement('div');
  taskEl.className = 'banner';
  taskEl.hidden = true;

  var lastItems = '';
  // Apps without a usable icon get a placeholder tile.
  document.addEventListener('error', function (e) {
    var t = e.target;
    if (t && t.id === 'jobIcon') { t.style.visibility = 'hidden'; return; }
    if (t && t.tagName === 'IMG' && t.className === 'icon') {
      var ph = document.createElement('div');
      ph.className = 'icon ph';
      ph.textContent = 'VPK';
      t.parentNode.replaceChild(ph, t);
    }
  }, true);

  function render() {
    if (!S) return;
    connection();
    banners();
    catalog();
    task();
    var items = S.items || [];
    $('add').className = 'drop' + (items.length ? ' compact' : '');
    $('dropTitle').textContent = items.length ? 'Add more' : 'Drop a Vita game here';
    var key = JSON.stringify([items, !!S.vita, S.vita && S.vita.companion, !!S.drive]);
    if (key !== lastItems) { lastItems = key; $('items').innerHTML = items.map(itemCard).join(''); }
    dock();
    job();
    document.title = S.job && S.job.progress && S.job.progress.total > 0 && S.job.progress.phase === 'send'
      ? Math.floor(S.job.progress.done * 100 / S.job.progress.total) + '% · CBOC Vita Installer' : 'CBOC Vita Installer';
  }

  // ---- actions ----
  document.addEventListener('click', function (e) {
    var t = e.target.closest ? e.target.closest('button') : null;
    if (!t) return;
    var d = t.dataset;
    if (d.remove) api('remove', { item: d.remove });
    else if (d.pickItem) api('pick', { item: d.pickItem, input: d.pickInput, kind: d.kind });
    else if (d.launch) api('launch', { item: d.launch });
    else if (d.get) { api('url', { url: 'https://github.com/' + d.get }); }
    else if (d.open) api('open', { url: d.open });
    else if (d.dismiss !== undefined) { localNotice = null; api('dismiss'); render(); }
    else if (d.host) { api('connect', { host: d.host }); }
    else if (d.drive) { api('connect', { drive: d.drive }); }
    else if (d.close !== undefined) closeSheets();
  });
  $('pickFile').onclick = function () { api('pick', { kind: 'file' }); };
  $('pickFolder').onclick = function () { api('pick', { kind: 'folder' }); };
  $('pickDrive').onclick = function () { api('pick', { kind: 'drive' }).then(closeSheets); };
  $('installBtn').onclick = function () { localNotice = null; api('install'); };
  $('stopBtn').onclick = function () { api('cancel'); };
  $('quitBtn').onclick = function () {
    api('quit').then(function () { document.body.innerHTML = '<main><h1>Closed</h1><p class="muted">You can close this tab.</p></main>'; });
  };
  $('conn').onclick = function () { $('connSheet').hidden = false; api('scan'); };
  $('urlForm').onsubmit = function (e) {
    e.preventDefault();
    var v = $('url').value.trim();
    if (v) api('url', { url: v }).then(function (j) { if (j && j.ok) $('url').value = ''; });
  };
  $('hostForm').onsubmit = function (e) {
    e.preventDefault();
    var v = $('host').value.trim();
    if (v) api('connect', { host: v }).then(function (j) { if (j && j.ok) closeSheets(); });
  };
  function closeSheets() { $('connSheet').hidden = true; }
  $('connSheet').addEventListener('click', function (e) { if (e.target === $('connSheet')) closeSheets(); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') closeSheets(); });

  // ---- drag and drop ----
  var overlay = $('overlay'), depth = 0;
  function hasFiles(e) {
    var t = e.dataTransfer && e.dataTransfer.types;
    return t && Array.prototype.indexOf.call(t, 'Files') >= 0;
  }
  document.addEventListener('dragenter', function (e) { if (!hasFiles(e)) return; e.preventDefault(); depth++; overlay.hidden = false; });
  document.addEventListener('dragleave', function () { depth = Math.max(0, depth - 1); if (!depth) overlay.hidden = true; });
  document.addEventListener('dragover', function (e) {
    if (!hasFiles(e)) return;
    e.preventDefault();
    var row = e.target.closest ? e.target.closest('[data-drop-item]') : null;
    Array.prototype.forEach.call(document.querySelectorAll('.need.dragover'), function (n) { if (n !== row) n.classList.remove('dragover'); });
    if (row) row.classList.add('dragover');
    overlay.firstChild.textContent = row ? 'Drop to use for this game' : 'Drop to add';
  });
  document.addEventListener('drop', function (e) {
    if (!hasFiles(e)) return;
    e.preventDefault();
    depth = 0;
    overlay.hidden = true;
    var row = e.target.closest ? e.target.closest('[data-drop-item]') : null;
    Array.prototype.forEach.call(document.querySelectorAll('.need.dragover'), function (n) { n.classList.remove('dragover'); });
    var dt = e.dataTransfer, list = [];
    for (var i = 0; i < dt.files.length; i++) {
      var entry = dt.items && dt.items[i] && dt.items[i].webkitGetAsEntry ? dt.items[i].webkitGetAsEntry() : null;
      list.push({ file: dt.files[i], entry: entry });
    }
    list.reduce(function (p, x) {
      return p.then(function () {
        return x.entry && x.entry.isDirectory ? uploadFolder(x.entry, row) : upload(x.file, row);
      });
    }, Promise.resolve()).then(function () { clientTask = null; render(); });
  });

  // Folders are read entry by entry and recreated next to the installer.
  var clientTask = null;
  function walk(entry, out) {
    if (entry.isFile) {
      return new Promise(function (ok) {
        entry.file(function (f) { out.push({ file: f, path: entry.fullPath.replace(/^\//, '') }); ok(); }, function () { ok(); });
      });
    }
    var reader = entry.createReader();
    return new Promise(function (ok) {
      var all = [];
      (function more() {
        reader.readEntries(function (batch) {
          if (!batch.length) {
            all.reduce(function (p, c) { return p.then(function () { return walk(c, out); }); }, Promise.resolve()).then(ok);
            return;
          }
          all = all.concat(Array.prototype.slice.call(batch));
          more();
        }, function () { ok(); });
      })();
    });
  }
  function uploadFolder(entry, row) {
    var group = (Date.now().toString(36) + Math.random().toString(36).slice(2, 10)).toLowerCase();
    var files = [];
    clientTask = 'Reading ' + entry.name + '…';
    render();
    return walk(entry, files).then(function () {
      files = files.filter(function (x) { return !/(^|\/)(\.DS_Store|\._[^/]*)$/.test(x.path); });
      var total = 0, done = 0, n = 0, failed = false;
      files.forEach(function (x) { total += x.file.size; });
      var next = 0;
      function worker() {
        if (failed || next >= files.length) return Promise.resolve();
        var x = files[next++];
        var q = 'name=' + encodeURIComponent(x.file.name) + '&group=' + group + '&rel=' + encodeURIComponent(x.path) + '&modified=' + (x.file.lastModified || '');
        return fetch(base + 'api/upload?' + q, { method: 'POST', headers: { 'X-Requested-With': 'cboc' }, body: x.file })
          .then(function (r) { return r.json(); })
          .then(function (j) {
            if (j && j.error) { failed = true; showNotice(j.error, true); return; }
            done += x.file.size; n++;
            clientTask = 'Reading ' + entry.name + ' · ' + n + ' of ' + files.length + ' files' + (total ? ' · ' + Math.floor(done * 100 / total) + '%' : '');
            task();
            return worker();
          });
      }
      return Promise.all([worker(), worker(), worker(), worker()]).then(function () {
        if (failed) return;
        clientTask = null;
        var b = { group: group, root: entry.name };
        if (row) { b.item = row.dataset.dropItem; b.input = row.dataset.dropInput; }
        return api('uploaded', { path: JSON.stringify(b) }).then(function (j) { if (j && j.ok) localNotice = null; render(); });
      });
    }).catch(function () { clientTask = null; showNotice('Could not read the folder ' + entry.name + '.', true); });
  }
  function upload(f, row) {
    var q = 'name=' + encodeURIComponent(f.name) + '&modified=' + (f.lastModified || '');
    if (row) q += '&item=' + encodeURIComponent(row.dataset.dropItem) + '&input=' + encodeURIComponent(row.dataset.dropInput);
    return fetch(base + 'api/upload?' + q, { method: 'POST', headers: { 'X-Requested-With': 'cboc' }, body: f })
      .then(function (r) { return r.json(); })
      .then(function (j) { if (j && j.error) showNotice(j.error, true); else localNotice = null; render(); })
      .catch(function () { showNotice('Could not read ' + f.name + '.', true); });
  }

  $('notice').parentNode.insertBefore(taskEl, $('notice').nextSibling);
  if (!Element.prototype.closest) {
    Element.prototype.closest = function (s) {
      var el = this;
      while (el && el.nodeType === 1) { if (el.matches ? el.matches(s) : el.msMatchesSelector(s)) return el; el = el.parentElement; }
      return null;
    };
  }
  connect();
})();
