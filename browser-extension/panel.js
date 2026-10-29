'use strict';
const engine = new UltraPrivacy.Client(chrome);
let records = [], busy = false, pending = null, fresh = false;
const drafts = Object.create(null);
const byId = id => document.getElementById(id);
const escapeHTML = text => String(text === undefined || text === null ? '' : text).replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
const help = (text, label) => '<button type="button" class="help" data-help="' + escapeHTML(text) + '" aria-label="' + escapeHTML((label || text.slice(0, 24)) + '：帮助') + '">?</button>';
const modes = { allow: '允许', ask: '询问', block: '拒绝' };
function originReadyFor(item, draft) {
  if (!item.allowRequiresOrigin || draft.value !== 'allow') return true;
  try {
    const url = new URL(draft.origin);
    return url.protocol === 'https:' && !url.username && !url.password && url.pathname === '/' && !url.search && !url.hash && !url.hostname.includes('*');
  } catch (error) { return false; }
}
function applyDisabled(item, draft) {
  return busy || !fresh || !item.supported || !item.writable || !originReadyFor(item, draft) || !!(item.stage && item.stage !== 'active');
}
function actionHintFor(item, draft) {
  return !fresh ? '请先重新读取；旧结果不能用于修改。' : item.unknown ? '当前无法确认，扩展不会猜测或强行修改。' : !item.supported ? '此浏览器未提供安全接口；这里只能说明边界。' : item.stage && item.stage !== 'active' ? '上次操作未完成，请先撤销本扩展覆盖并重新检查。' : item.type === 'privacy' && !item.writable ? '企业策略或其他扩展拥有控制权，本扩展不会覆盖。' : !originReadyFor(item, draft) ? '摄像头/麦克风选择“允许”时，请填写明确的 HTTPS 站点原点。' : '';
}
function message(text, error) { byId('message').textContent = text; byId('message').className = error ? 'error' : ''; }
function decorateHelp() {
  document.querySelectorAll('button:not(.help),input,textarea,select,summary').forEach(control => {
    const label = control.getAttribute('aria-label') || control.textContent.trim() || control.id;
    const next = control.nextElementSibling;
    if (next && next.classList.contains('help')) { next.setAttribute('aria-label', label + '：帮助'); return; }
    const details = control.matches('textarea') ? '内容仅在本页面处理，不自动打开网址、上传或复制；输入变化后需要重新预览。' : control.matches('select,input') ? '待应用。选择或输入不会修改浏览器；另行确认后执行。来源只接受明确 HTTPS 原点，不含路径。' : control.hasAttribute('data-tab') ? '切换查看范围，不修改浏览器设置；取消未提交的确认。' : control.id === 'cancel-pending' ? '取消尚未提交的修改，不撤销已开始的操作。' : '只操作此项；设置与撤销经共享后台串行处理，结果未知时不自动重试。';
    control.insertAdjacentHTML('afterend', help(details, label));
  });
}
function draw() {
  byId('refresh').disabled = busy;
  byId('controls').innerHTML = records.map(item => {
    const draft = drafts[item.id] || (drafts[item.id] = { value: item.appliedValue || item.desired, origin: item.origin || '' });
    const status = !fresh ? '历史结果，当前未确认' : item.unknown ? '本次无法读取' : !item.supported ? '此浏览器不支持' : item.stage && item.stage !== 'active' ? '操作未完成，可撤销覆盖' : item.orphaned ? '发现未记录的本扩展覆盖' : item.type === 'privacy' && !item.writable ? '其他策略或扩展控制' : item.type === 'content' ? '仅此来源抽样结果' : item.control === 'controlled_by_this_extension' ? '上次读回由本扩展控制' : '上次读回未由本扩展控制';
    const control = { not_controllable: '企业策略或浏览器限制', controlled_by_other_extensions: '其他扩展', controllable_by_this_extension: '本扩展可请求控制', controlled_by_this_extension: '本扩展控制' }[item.control] || (item.type === 'content' ? 'API不提供统一控制权字段' : '未知');
    const originLabel = item.allowRequiresOrigin ? 'HTTPS 站点来源（允许必须填写；拒绝/询问可留空）' : 'HTTPS 站点来源（留空作用于全部站点）';
    const options = item.type === 'content' ? `<label for="mode-${escapeHTML(item.id)}">权限模式（待应用）</label><select id="mode-${escapeHTML(item.id)}" data-mode="${escapeHTML(item.id)}" ${busy ? 'disabled' : ''}>${item.values.map(value => `<option value="${value}" ${draft.value === value ? 'selected' : ''}>${modes[value]}</option>`).join('')}</select><label for="origin-${escapeHTML(item.id)}">${originLabel}</label><input id="origin-${escapeHTML(item.id)}" data-origin="${escapeHTML(item.id)}" type="url" placeholder="https://example.invalid" value="${escapeHTML(draft.origin)}" ${busy ? 'disabled' : ''}>${item.allowRequiresOrigin ? '<p class="small muted">浏览器不允许摄像头/麦克风全站“允许”；请填明确 HTTPS 站点原点。</p>' : ''}` : '';
    const restore = item.managed || item.orphaned || item.type === 'content';
    const applyLabel = item.type === 'content' ? '审阅所选模式' : item.managed ? '重新应用并读回' : '应用并读回';
    const sampleNote = item.type === 'content' ? '内容权限 API 只返回一个来源的抽样值；站点例外、iframe 和 Worker 仍需单独核对。' : '仅当前正常浏览配置；读回值不等于所有通信已受限。';
    const originReady = originReadyFor(item, draft);
    const actionHint = actionHintFor(item, draft);
    return `<article><div class="card-head"><h2>${escapeHTML(item.title)} ${help(item.impact, item.title)}</h2><span class="badge">${status}</span></div><p class="small">${item.supported ? '上次读回值：' + escapeHTML(String(item.value)) : escapeHTML(item.detail || '无法确认')}</p><details><summary>范围、影响与依据</summary><p class="small muted">${escapeHTML(item.impact)}</p><p class="small">控制权：${escapeHTML(control)}。${sampleNote}</p><p class="small muted">上次检查：${escapeHTML(item.checkedAt || '未提供')}。</p></details>${options}<div class="actions"><button class="primary" data-request="${escapeHTML(item.id)}" ${busy || !fresh || !item.supported || !item.writable || !originReady || (item.stage && item.stage !== 'active') ? 'disabled' : ''}>${applyLabel}</button>${restore ? `<button data-restore="${escapeHTML(item.id)}" ${busy || !fresh || (!item.supported && !item.clearable) ? 'disabled' : ''}>撤销本扩展覆盖</button>` : ''}</div>${actionHint ? `<p class="small muted action-hint">${escapeHTML(actionHint)}</p>` : ''}${pending && pending.id === item.id ? `<div class="notice"><p>${pending.action === 'apply' ? '待应用：' + (item.type === 'content' ? escapeHTML(modes[pending.options.value]) + '；' + escapeHTML(pending.options.origin || '全部站点') + '。' : '') + escapeHTML(item.impact) : '待撤销：只清除本扩展此类规则，不覆盖用户、企业或其他扩展的当前设置。内容权限 API 不能证明所有覆盖均已撤销。'}</p><button data-confirm="${escapeHTML(item.id)}">${pending.action === 'apply' ? '确认应用并读回' : '确认撤销'}</button><button id="cancel-pending">取消</button></div>` : ''}</article>`;
  }).join('');
  byId('controls').querySelectorAll('[data-mode]').forEach(input => input.addEventListener('change', () => { drafts[input.dataset.mode].value = input.value; pending = null; draw(); }));
  byId('controls').querySelectorAll('[data-origin]').forEach(input => input.addEventListener('input', () => {
    const id = input.dataset.origin, draft = drafts[id];
    draft.origin = input.value;
    // Keep the actual input and caret while immediately re-evaluating the
    // action. Rebuilding only on a later tab change left valid origins stuck.
    if (pending) {
      pending = null;
      byId('controls').querySelectorAll('[data-confirm]').forEach(button => { const notice = button.closest('.notice'); if (notice) notice.remove(); });
    }
    const item = records.find(record => record.id === id), article = input.closest('article');
    if (!item || !article) return;
    article.querySelector('[data-request]').disabled = applyDisabled(item, draft);
    const text = actionHintFor(item, draft);
    let hint = article.querySelector('.action-hint');
    if (!text) { if (hint) hint.remove(); }
    else { if (!hint) { hint = document.createElement('p'); hint.className = 'small muted action-hint'; article.appendChild(hint); } hint.textContent = text; }
  }));
  byId('controls').querySelectorAll('[data-request]').forEach(button => button.addEventListener('click', () => { const draft = drafts[button.dataset.request]; pending = { id: button.dataset.request, action: 'apply', options: { value: draft.value, origin: draft.origin } }; draw(); }));
  byId('controls').querySelectorAll('[data-restore]').forEach(button => button.addEventListener('click', () => { pending = { id: button.dataset.restore, action: 'restore' }; draw(); }));
  byId('controls').querySelectorAll('[data-confirm]').forEach(button => button.addEventListener('click', () => execute(button.dataset.confirm)));
  const cancel = byId('cancel-pending'); if (cancel) cancel.addEventListener('click', () => { pending = null; draw(); });
  decorateHelp();
}
async function refresh() {
  if (busy) return;
  busy = true; fresh = false; pending = null; draw();
  try { records = await engine.report(); fresh = true; message('已在本地重新读取。没有访问检测网站。'); }
  catch (error) { message('读取失败，历史结果已失效：' + error.message, true); }
  busy = false; draw();
}
async function execute(id) {
  if (busy || !fresh || !pending || pending.id !== id) return;
  const action = pending.action, options = pending.options;
  busy = true; fresh = false; pending = null; draw();
  try {
    const result = await engine[action](id, options);
    records = await engine.report(); fresh = true;
    message(action === 'restore' ? result.verified ? '已读回确认不再由本扩展控制；底层设置由其他来源决定。' : '浏览器已确认清除本扩展此类规则；仅有抽样读回，仍需实际站点核对。' : result.sample ? '已提交所选模式并抽样读回；不代表所有站点与上下文。' : '读回值与本扩展控制权符合目标。');
  } catch (error) {
    try { records = await engine.report(); fresh = true; } catch (readError) { fresh = false; }
    message('操作未完成或结果未知：' + error.message + ' 不自动重试；先重新检查，必要时撤销本扩展覆盖。', true);
  }
  busy = false; draw();
}
document.querySelectorAll('[data-tab]').forEach(button => button.addEventListener('click', () => {
  pending = null; draw();
  document.querySelectorAll('[data-tab]').forEach(tab => { tab.classList.toggle('active', tab === button); byId(tab.dataset.tab).hidden = tab !== button; });
}));
byId('refresh').addEventListener('click', refresh);
function invalidate() { fresh = false; pending = null; draw(); }
window.addEventListener('focus', invalidate);
if (chrome.storage && chrome.storage.onChanged) chrome.storage.onChanged.addListener((changes, area) => { if (area === 'local' && changes.managed) invalidate(); });

byId('run-probe').addEventListener('click', async () => {
  const button = byId('run-probe'); button.disabled = true;
  byId('probe-result').innerHTML = '';
  const result = {};
  function record(name, value, detail) { result[name] = { value: String(value), detail: detail }; }
  try {
    record('检查范围', '本浏览器扩展页面', '是真实浏览器返回值，但扩展来源、站点权限、iframe 和 Worker 上下文可能不同。');
    record('时区', Intl.DateTimeFormat().resolvedOptions().timeZone || '未返回', '网页可从日期接口推测时区；修改系统或使用受支持浏览器的统一保护后需要复测。');
    record('语言', navigator.languages.join(', '), '语言与出口国家不一致可能构成特征。不要随意随机化每次返回值。');
    record('User Agent', navigator.userAgent, '这是当前上下文可见的浏览器标识，UA Client Hints 可能另外提供信息。');
    record('平台 / 触摸点', navigator.platform + ' / ' + navigator.maxTouchPoints, '网页可据此推测平台和输入设备，扩展未伪造这些值。');
    record('屏幕 / DPR', screen.width + ' × ' + screen.height + ' / ' + devicePixelRatio, '窗口、缩放和显示器配置会影响此特征。');
    record('逻辑处理器 / 内存估计', String(navigator.hardwareConcurrency || '未提供') + ' / ' + (navigator.deviceMemory ? navigator.deviceMemory + ' GB' : '未提供'), '这些是浏览器公开的估计或裁剪值，不等于物理硬件全量信息。');
    record('Canvas', document.createElement('canvas').getContext('2d') ? '2D 接口可用' : '2D 接口不可用', '没有生成、哈希、保存或上传稳定的 Canvas 指纹；接口可用不代表已发生跟踪。');
    record('字体', document.fonts ? 'FontFaceSet 接口存在，未枚举字体' : '字体集接口不可用', '不枚举系统字体清单；网页仍可能根据布局差异推测字体。普通扩展无法统一屏蔽所有字体指纹。');
    record('Audio / WebRTC', 'Audio ' + (window.AudioContext ? '接口存在' : '不可用') + '；WebRTC ' + (window.RTCPeerConnection ? '接口存在' : '不可用'), '不播放声音，不计算音频指纹，不创建 WebRTC 连接，也不请求 STUN / TURN。');
    let gl;
    try {
      gl = document.createElement('canvas').getContext('webgl');
      const ext = gl && gl.getExtension('WEBGL_debug_renderer_info');
      record('WebGL 渲染器', !gl ? '上下文不可用' : ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : '可用；未开放调试渲染器名称', '实际读到的 GPU 渲染器信息。仅关闭硬件加速不等于 WebGL 和 GPU 指纹都已消失。');
    } catch (error) { record('WebGL 渲染器', '读取失败：' + error.message, '失败不代表所有上下文已关闭 GPU 能力。'); }
    finally { if (gl) { const lose = gl.getExtension('WEBGL_lose_context'); if (lose) lose.loseContext(); } }
    record('WebGPU', navigator.gpu ? '接口存在，未创建设备或读取高熵适配器信息' : '接口不可用', '接口状态随上下文、安全环境、浏览器版本和策略变化；不把不可用推断为整机受保护。');
    for (const name of ['geolocation', 'camera', 'microphone', 'notifications', 'clipboard-read']) {
      try {
        const info = await navigator.permissions.query({ name: name });
        record('此扩展来源权限：' + name, info.state, '只查询状态，不请求授权。普通网站有自己的权限状态，应在该站点的浏览器设置中确认。');
      } catch (error) { record('此扩展来源权限：' + name, '浏览器不支持此查询或当前来源不可查询', '无法确认不能显示为已拒绝。'); }
    }
    byId('probe-result').innerHTML = '<dt>本次运行时间</dt><dd>' + escapeHTML(new Date().toISOString()) + '</dd>' + Object.keys(result).map(key => '<dt>' + escapeHTML(key) + ' ' + help(result[key].detail, key) + '</dt><dd>' + escapeHTML(result[key].value) + '</dd>').join('');
    message('本地自测完成。结果只在此页面内存中，未外发或保存。');
  } catch (error) { message('本地自测未完成：' + error.message, true); }
  button.disabled = false;
});
byId('clean-link').addEventListener('click', () => {
  try {
    const result = UltraCleanLink(byId('link-input').value);
    byId('link-output').value = result.url;
    byId('link-detail').textContent = result.reason + (result.removed.length ? ' 已移除：' + result.removed.join(', ') : '');
    byId('copy-link').disabled = !result.url;
  } catch (error) { byId('link-detail').textContent = error.message; byId('link-output').value = ''; byId('copy-link').disabled = true; }
});
byId('link-input').addEventListener('input', () => { byId('link-output').value = ''; byId('link-detail').textContent = '待清理：输入变化后，需重新预览。'; byId('copy-link').disabled = true; });
byId('copy-link').addEventListener('click', async () => {
  try { await navigator.clipboard.writeText(byId('link-output').value); message('已复制清理结果。'); }
  catch (error) { byId('link-output').select(); message('浏览器未允许写入剪贴板，请手动复制已选中的结果。', true); }
});
// Lightweight tooltips: hover / focus / click to pin, Escape / outside click to close.
let tipAnchor = null, pinned = false;
function closeTip() { if (tipAnchor) tipAnchor.removeAttribute('aria-describedby'); tipAnchor = null; pinned = false; byId('tooltip').hidden = true; }
function showTip(anchor, pin) {
  closeTip(); tipAnchor = anchor; pinned = pin;
  const tip = byId('tooltip'); tip.textContent = anchor.dataset.help; tip.hidden = false; anchor.setAttribute('aria-describedby', 'tooltip');
  tip.style.maxWidth = Math.min(290, innerWidth - 20) + 'px';
  const rect = anchor.getBoundingClientRect(), size = tip.getBoundingClientRect();
  tip.style.left = Math.max(8, Math.min(rect.left - size.width / 2, innerWidth - size.width - 8)) + 'px';
  tip.style.top = Math.max(8, rect.top > size.height + 12 ? rect.top - size.height - 6 : Math.min(rect.bottom + 6, innerHeight - size.height - 8)) + 'px';
}
document.addEventListener('mouseover', event => { const button = event.target.closest('[data-help]'); if (button && button !== tipAnchor) showTip(button, false); });
document.addEventListener('mouseout', event => { if (!pinned && tipAnchor && event.target.closest('[data-help]') === tipAnchor && !tipAnchor.contains(event.relatedTarget)) closeTip(); });
document.addEventListener('focusin', event => { const button = event.target.closest('[data-help]'); if (button) showTip(button, false); else if (!pinned) closeTip(); });
document.addEventListener('focusout', () => { if (!pinned) closeTip(); });
document.addEventListener('click', event => { const button = event.target.closest('[data-help]'); if (!button) closeTip(); else if (tipAnchor === button && pinned) closeTip(); else showTip(button, true); });
document.addEventListener('keydown', event => { if (event.key === 'Escape') closeTip(); });
document.addEventListener('scroll', closeTip, true); window.addEventListener('resize', closeTip);
refresh();
