// Optional privacy operations never call the tunnel lifecycle.
const privacyWork = { busy: false, epoch: 0 };
function privacyEvidenceChanged() { privacyWork.epoch++; document.dispatchEvent(new Event('ultra-privacy-changed')); }
function privacyGuardDiagnosticText(diagnostic) {
  const stages = ['manager_unavailable','owner_unavailable','owner_process_id','owner_process_times','owner_process_open','owner_process_identity','owner_token_open','owner_token_identity','owner_token_elevation','owner_known_folder','owner_local_drive','owner_debug_thread','owner_debug_token','owner_debug_duplicate','owner_debug_identity','owner_debug_enable','owner_debug_attach','owner_debug_open','owner_debug_revert','owner_debug_retire','lease_job_duplicate','lease_job_limits','lease_job_processes','lease_process_open','lease_process_identity','lease_directory','lease_image','lease_receipt','lease_code_pin','lease_code_seal','request_validation','existing_session','wfp_install','wfp_verify','wfp_remove','job_query','job_active','guard_report','guard_transport'];
  if (!diagnostic || !stages.includes(diagnostic.stage) || !Number.isInteger(diagnostic.nativeCode) || diagnostic.nativeCode < 0 || diagnostic.nativeCode > 4294967295) return '';
  return '阶段 ' + diagnostic.stage + ' · 系统代码 ' + (diagnostic.nativeCode === 0 ? '未提供' : diagnostic.nativeCode);
}
function privacyGuideModules(catalog) {
  return catalog && catalog.schemaVersion === 1 && catalog.kind === 'help_only_not_live_evidence' && Array.isArray(catalog.modules) ? catalog.modules.slice(0, 32).filter(module => module && typeof module.id === 'string' && typeof module.title === 'string' && typeof module.summary === 'string') : [];
}
function privacyGuideActionAllowed(action) {
  const targets = {settings:['privacy','location','privacy-location','camera','microphone','notifications','background','diagnostics','advertising','privacy-general','activity','language','time','dateandtime','wifi','network-wifi','browser','firewall','windowsdefender','privacy-speech','clipboard'], 'privacy-tab':['system','network','browser','data','boundaries'], 'app-page':['settings','logs','profiles','detection','privacy']};
  return !!(action && targets[action.kind] && targets[action.kind].includes(action.target) && typeof action.label === 'string');
}
function privacyModuleGuideCards(catalog, ids) {
  const names = {backend:'已有受控操作', limited:'部分范围', settings:'系统入口', pending:'待接通'};
  return ids.map(id => privacyGuideModules(catalog).find(module => module.id === id)).filter(Boolean).map(module => `<article class="privacy-card privacy-module-guide" data-privacy-guide="${esc(module.id)}"><details><summary><strong>${esc(module.title)}</strong><span class="privacy-badge">${esc(names[module.capability] || '范围说明')}</span><span class="privacy-guide-summary">${esc(module.summary)}</span></summary><p class="muted small">以下是功能说明，当前状态以本页实际检查为准。</p><dl><dt>覆盖范围</dt><dd>${esc(module.scope || '范围未提供')}</dd><dt>如何核对</dt><dd>${esc(module.verification || '仍需实际读回')}</dd><dt>如何恢复</dt><dd>${esc(module.recovery || '没有自动恢复操作')}</dd>${(Array.isArray(module.limitations) ? module.limitations : []).map(value => `<dt>边界</dt><dd>${esc(value)}</dd>`).join('')}${(Array.isArray(module.pending) ? module.pending : []).map(value => `<dt>未完成项</dt><dd>${esc(value)}</dd>`).join('')}</dl><div class="privacy-actions">${(Array.isArray(module.actions) ? module.actions : []).filter(privacyGuideActionAllowed).map(action => `<button type="button" class="btn sm" data-privacy-guide-action="${esc(action.kind)}" data-guide-target="${esc(action.target)}">${esc(action.label)}</button>`).join('')}</div></details></article>`).join('');
}
function privacyPageContext(el, page, invalidate, dispose) {
  let pending = false, closed = false, generation = 0;
  const alive = () => !closed && typeof document !== 'undefined' && document && document.documentElement && document.documentElement.contains(el) && !el.classList.contains('pop') && (typeof view === 'undefined' || view === page);
  const changed = () => { if (alive()) invalidate(); };
  document.addEventListener('ultra-privacy-changed', changed);
  const observer = new MutationObserver(() => {
    if (alive()) return;
    closed = true; observer.disconnect(); if(typeof dispose==='function')dispose(); if (typeof document !== 'undefined' && document) document.removeEventListener('ultra-privacy-changed', changed);
    window.removeEventListener('focus', changed);
    if (pending && typeof closeDialog === 'function') closeDialog(false);
  });
  observer.observe(document.documentElement, { childList: true, subtree: true });
  window.addEventListener('focus', changed);
  return { alive, cancelPending: () => { generation++; if (pending && typeof closeDialog === 'function') closeDialog(false); }, confirm: async (text, options) => {
    if (!alive() || privacyWork.busy) return false;
    const epoch = privacyWork.epoch;
    const ownGeneration = generation;
    pending = true;
    const ok = await askConfirm(text, options);
    pending = false;
    return !!ok && alive() && epoch === privacyWork.epoch && ownGeneration === generation && !privacyWork.busy;
  } };
}
function renderPrivacy(el, arg) {
  const model = { report: null, firewall: null, host: null, tab: 'system', category: '', search: '', busy: false, loading: false, error: '', firewallError: '', program: '', mode: 'block_lan', probe: null, checked: '', dataReport: null, dataAction: '', dataChecked: '', time: null, timeReadable: false, timeDraft: null, timeError: '', timeMessage: '' };
  model.reportReadable = false; model.firewallReadable = false; model.helper = null; model.guides = null;
  model.browser = null; model.browserPreview = null; model.browserSeed = ''; model.browserBusy = false; model.browserError = ''; model.browserMessage = '';
  model.browserPane = 'environment'; model.browserIdentityPreview = null; model.browserIdentityGeneration = 0;
  model.browserStart=null;model.browserStartGeneration=0;model.browserStartCancelBusy=false;
  model.browserEngine={engineId:'webview2',version:''};model.browserRoute='ultra';
  model.browserProfileName='';model.browserSavedRecord=null;model.browserSavedDirty=false;model.browserSavedList=null;model.browserSavedSelection='';model.browserProfileGeneration=0;
  model.browserMeta={browser:'chromium',environmentName:'',group:'',remark:'',tags:[]};
  model.browserExtras={storage:{cookieFormat:'json',merge:false,cookieCount:0,cookieSessionOnly:true},tabs:{urls:[],startupMode:'restore'},proxy:{mode:'no-proxy',type:'none',provider:'IP2Location',accountRef:''},fingerprint:{webrtc:'disable-non-proxied-udp',timezone:'real',geo:'real',language:'real',uiLanguage:'by-language',resolution:'ua',fonts:'default',noise:{canvas:false,webglImage:false,audio:false,media:false,clientRects:false,speechVoices:false},webglMetadata:'real',webgpu:'real',seed:''},advanced:{cpu:'real',ram:'real',deviceName:'real',mac:'real',dnt:'default',portScan:'enabled',hardwareAcceleration:'default',tls:'default',teamApps:'global',dataSync:'global',browserSettings:'global',randomFingerprint:false},readback:{state:'unknown',scope:'browser-profile',runtimeVerified:false,warnings:[]}};
  model.browserCookieDraft=''; model.browserV2Message=''; model.browserV2Error='';
  model.browserProxy = {type:'socks5',host:'',port:1080,username:'',password:''}; model.browserProxyResult = null; model.browserProxyBusy = false; model.browserProxyError = ''; model.browserProxyGeneration = 0;
  model.metadataStatus = null; model.metadataPreview = null; model.metadataSelected = []; model.metadataValues = null; model.metadataCopy = null; model.metadataBusy = false; model.metadataError = ''; model.metadataGeneration = 0;
  model.portalDraft={latitude:'',longitude:'',label:'',trajectory:'',intervalSeconds:'2',port:'58443',certificateSHA256:'',username:'',password:''};model.portalReport=null;model.portalPreparation=null;model.portalRuntime=null;model.portalBusy=false;model.portalError='';model.portalMessage='';model.portalGeneration=0;model.portalPollGeneration=0;
  model.engineKind='chrome-for-testing';model.engineVersion='';model.engineRoute='ultra';model.engineCatalog=null;model.engineVersions=null;model.engineBusy=false;model.engineError='';model.engineMessage='';model.engineDeleteGeneration=0;model.engineDeleting=false;
  if (arg && arg.tab === 'browser') model.tab = 'browser';
  const cancelPortalObservation=()=>{if(!model.portalObservationPending)return;model.portalObservationPending=false;try{const bridge=App();if(bridge&&typeof bridge.PrivacyLocationPortalCancelObservation==='function')Promise.resolve(bridge.PrivacyLocationPortalCancelObservation()).catch(()=>{});}catch{}};
  const context = privacyPageContext(el, 'privacy', () => { model.reportReadable = false; model.firewallReadable = false; model.timeReadable = false; draw(); },()=>{cancelPortalObservation();model.portalPollGeneration++;if(model.regionCleanup){model.regionCleanup();model.regionCleanup=null;}});
  const alive = context.alive;
  const safe = value => esc(value === undefined || value === null ? '' : value);
  const guideCards = ids => privacyModuleGuideCards(model.guides, ids);
  const tabGuides = () => guideCards(({system:['system-policies','system-timezone','system-language-region','app-permissions'],network:['program-network','wifi-mac'],data:['image-metadata','secret-clipboard','diagnostics-logs'],boundaries:['native-hardware','location-provider','credential-storage','recovery','detection-observations']})[model.tab] || []);
  const centerSummary = () => { const all=controls().filter(control => ['windows','browser'].includes(control.category)), pending=all.filter(control => !model.reportReadable || ['unknown','unavailable','error','backup_error','recovery_pending'].includes(control.status) || control.readStatus === 'unknown').length; return `<div class="privacy-summary" aria-label="隐私设置检查概览"><div class="privacy-summary-item"><strong>${safe(all.length)}</strong><span>本次策略项目</span></div><div class="privacy-summary-item${pending ? ' attention' : ''}"><strong>${safe(pending)}</strong><span>待确认项目</span></div><div class="privacy-summary-item privacy-summary-time"><strong>${safe(model.reportReadable ? model.checked || '已读取' : '尚未确认')}</strong><span>本地策略检查时间</span></div></div>`; };
  const guardDiagnostic = () => { const guard=model.helper && model.helper.browserGuard, text=guard && !guard.available && privacyGuardDiagnosticText(guard.diagnostic); return text ? `<details class="privacy-note warning" id="privacy-browser-guard-diagnostic"><summary>专用浏览器网络隔离尚未就绪</summary><p>${safe(text)}</p><p>基础管理员助手可以已连接；这不代表浏览器隔离可用。未自动重试启动或更改系统规则。</p></details>` : ''; };
  const api = async (method, arg, extra) => {
    if (method === 'PrivacyOpenSettings' && arg === 'browser') { context.cancelPending();cancelPortalObservation();model.portalGeneration++;model.portalPollGeneration++; model.tab = 'browser'; draw(); return; }
    if (window.__web || window.__android || !state || state.platform !== 'windows') throw new Error('隐私中心仅支持 Windows 桌面客户端。');
    const bridge = App();
    if (!bridge || typeof bridge[method] !== 'function') throw new Error('当前客户端没有此隐私接口，请使用完整的 Ultra Windows 构建。');
    const writes = ['PrivacyApply', 'PrivacyRestore', 'PrivacyRestoreAll', 'PrivacyFirewallApply', 'PrivacyFirewallRemove', 'PrivacyTimeApply', 'PrivacyTimeRestore', 'PrivacyAuthorize','PrivacyRegionApply','PrivacyRegionRestore','PrivacyLocationPortalApply','PrivacyLocationPortalRestore','PrivacyLocationPortalPrepare','PrivacyLocationPortalRestorePreparation','PrivacyLocationPortalStopTrajectory'];
    const invoke = () => arg === undefined ? bridge[method]() : extra === undefined ? bridge[method](arg) : bridge[method](arg, extra);
    if (writes.indexOf(method) < 0) return invoke();
    if (!alive() || privacyWork.busy) throw new Error('另一个隐私操作正在处理，或操作页面已失效。');
    privacyWork.busy = true; privacyEvidenceChanged();
    try { return await invoke(); }
    finally { privacyWork.busy = false; privacyEvidenceChanged(); }
  };
  const label = control => {
    // Keep badges short while matching the evidence. A matching policy value
    // is configuration evidence only; it is not runtime protection.
    const labels = { filters_installed: '规则已装，流量未测', installed_unverified: '已提交，待复核', configured: '策略匹配，待运行验证', policy_conflict: '被其他策略覆盖', recovery_pending: '恢复未完成', external_change: '检测到外部更改', planned: '本版未实现', implemented: '已实现此边界', limited: '仅覆盖部分范围', enabled: '读回匹配', applied: '读回匹配', protected: '读回匹配', on: '读回匹配', disabled: '未启用', off: '未启用', available: '可应用', inactive: '未启用', manual: '需手动设置', unsupported: '本环境不支持', unavailable: '当前不可用', unknown: '无法确认', conflict: '设置有冲突', error: '检查失败', managed: '本应用已管理', blocked: '被其他策略限制', partial: '部分规则匹配', pending_restart: '需重启验证' };
    if (control.status === 'requires_admin') return '需要管理员授权';
    if (control.status === 'backup_error') return '备份不可用';
    return labels[control.status] || (control.status ? '无法确认' : '尚未检查');
  };
  const controls = () => (model.report && Array.isArray(model.report.controls) ? model.report.controls : []).map(control => model.reportReadable ? control : Object.assign({}, control, { status: 'unknown', readStatus: 'unknown', canApply: false, detail: '历史结果，当前未确认。请重新检查后操作。' }));
  const helperReady = () => model.helper && model.helper.state === 'connected' && model.reportReadable && model.report && model.report.admin === true;
  const firewallReady = () => model.firewallReadable && helperReady() && model.firewall && model.firewall.supported !== false && model.firewall.elevated !== false;
  const duplicateRule = () => model.firewall && Array.isArray(model.firewall.rules) && model.firewall.rules.some(entry => { const rule = entry.rule || entry; return String(rule.program || '').toLowerCase() === model.program.toLowerCase() && rule.mode === model.mode; });
  const canApply = control => model.reportReadable && control.canApply === true && (!control.requiresAdmin || helperReady());
  const actionHint = control => {
    if (!model.reportReadable) return '请先重新检查；旧结果不能用于修改。';
    if (control.status === 'unknown' || control.readStatus === 'unknown') return '当前无法确认，应用不会猜测或强行修改。';
    if (control.status === 'manual') return control.requiresAdmin && !helperReady() ? '需要先授权管理员助手；取消授权不会影响隧道。' : '本版不自动修改此项，请按下方系统设置说明处理。';
    if (control.status === 'unsupported') return '当前系统或浏览器不提供安全修改接口；这里只给出替代方案。';
    if (control.status === 'policy_conflict') return '检测到更高优先级策略，应用不会覆盖；请在组织或浏览器策略中处理。';
    if (control.status === 'recovery_pending') return '上次操作未完成，请先恢复原值，再重新检查。';
    if (control.status === 'external_change') return '外部软件已改动此项；应用不会覆盖新值，请先处理冲突。';
    if (control.status === 'configured' && !control.managed) return '目标值已存在，但不是本应用写入；不会提供恢复按钮或覆盖它。';
    if (control.canApply !== true) return '当前条件不满足自动修改；请查看“范围、证据与恢复”。';
    return '';
  };
  const categoryName = value => ({ windows: 'Windows 系统', browser: '浏览器策略', guidance: '手动引导' }[value] || value || '系统设置');
  const restartText = control => typeof control.restart === 'string' ? control.restart : control.restart ? '需要完全退出并重启目标浏览器或应用，再验证实际效果；系统项目可能需要重新登录或重启。' : '修改后重新检查；已启动的应用可能缓存原值。';
  const readEvidence = control => ({ match: '只读证据：固定策略值符合目标；适用性和写入权限另行判断。', different: '只读证据：固定策略值未设置或不同；不表示已发生信息泄露。', machine_override: '只读证据：存在机器级策略覆盖。', unknown: '只读证据：本次无法确认。', not_checked: '只读证据：尚未核对。' }[control.readStatus] || '');
  const timeChoices = () => model.time && Array.isArray(model.time.choices) ? model.time.choices.filter(choice => choice && typeof choice.id === 'string' && choice.id && typeof choice.label === 'string') : [];
  const timeCard = () => {
    const time = model.time, choices = timeChoices();
    const usable = model.timeReadable && time && time.supported && time.currentId && choices.some(choice => choice.id === time.currentId) && choices.some(choice => choice.id === model.timeDraft);
    const canSet = usable && !model.timeError && time.canApply !== false && ['available', 'configured'].indexOf(time.status) >= 0;
    const timeBadge = !model.timeReadable ? '当前未确认' : !usable ? '仅系统引导' : time.status === 'configured' ? '本应用已管理，仍需浏览器复核' : time.status === 'backup_error' ? '备份不可用' : label(time);
    return `<article class="privacy-card" id="privacy-time-card"><div class="privacy-card-head"><h2>系统时区${privacyHelp('从本机 Windows 枚举出的真实时区中选择。只修改系统时区，保留夏令时偏好；不修改 UTC 时钟、语言、键盘或隧道，不根据出口 IP 自动猜测。')}</h2><span class="privacy-badge">${safe(timeBadge)}</span></div><p>选择后点击应用才会修改。日历和时间显示可能变化；浏览器可能覆盖系统时区，语言与地区需要另行检查。</p>${time && time.currentId ? `<dl><dt>${model.timeReadable ? '当前系统时区 ID' : '上次读回时区 ID（当前未确认）'}</dt><dd>${safe(time.currentId)}</dd><dt>${model.timeReadable ? '当前夏令时偏好' : '上次读回夏令时偏好（当前未确认）'}</dt><dd>${time.daylightSavingDisabled ? '自动调整已关闭' : '允许按系统规则自动调整'}（本工具保留此设置）</dd></dl>` : ''}${time && time.detail ? `<p class="muted">${model.timeReadable ? '' : '上次报告：'}${safe(time.detail)}</p>` : ''}${usable ? `<label class="privacy-time-label" for="privacy-time-choice">选择系统时区</label><select id="privacy-time-choice" data-help-text="待应用。选项只来自当前 Windows 时区列表，不自动跟随出口。更改选择不会修改系统，点击应用并确认后才执行。" ${model.busy || model.loading ? 'disabled' : ''}>${choices.map(choice => `<option value="${safe(choice.id)}" ${model.timeDraft === choice.id ? 'selected' : ''}>${safe(choice.label)}</option>`).join('')}</select><p class="privacy-pending">${model.timeDraft === time.currentId ? '当前选择与系统一致；尚未提交新的更改。' : '待应用：已选择其他时区，系统尚未修改。'}</p>` : '<p class="muted">尚无可验证的时区选项；使用 Windows 设置，不提供猜测的列表。</p>'}${model.timeError ? `<p class="privacy-error" role="alert">${safe(model.timeError)}</p>` : ''}${model.timeMessage ? `<p class="small muted">${safe(model.timeMessage)}</p>` : ''}<div class="privacy-actions">${usable ? `<button class="btn sm primary" id="privacy-time-apply" data-help-text="确认后仅应用选定时区；保存原时区和夏令时偏好，然后重新读取当前 ID。任何失败都不会断开隧道或修改其他保护。" ${!canSet || model.timeDraft === time.currentId || model.busy || model.loading ? 'disabled' : ''}>应用所选时区</button>` : ''}<button class="btn sm" id="privacy-time-refresh" data-help-text="仅重新读取当前时区 ID、夏令时偏好和支持列表，不自动应用待选项。" ${model.busy || model.loading ? 'disabled' : ''}>重新读取时区</button>${time && time.managed ? `<button class="btn sm" id="privacy-time-restore" data-help-text="单独恢复本应用保存的原时区和夏令时偏好，之后重新读取。检测到外部修改时不覆盖，需按提示处理。" ${model.busy || model.loading || !model.timeReadable || time.canApply === false ? 'disabled' : ''}>恢复原时区</button>` : ''}<button class="btn sm" data-privacy-settings="dateandtime" data-help-text="打开 Windows 日期和时间设置。这里只提供固定系统入口，打开页面不代表设置已修改。">Windows 时间设置</button></div></article>`;
  };
  const controlCard = control => `<article class="privacy-card" data-privacy-control="${safe(control.id)}">
    <div class="privacy-card-head"><h2>${safe(control.title || control.id)}${privacyHelp(control.impact || '查看范围与证据后按需修改。', control.title || control.id)}</h2><span class="privacy-badge ${control.managed ? 'managed' : ''}">${safe(label(control))}</span></div>
    <div class="privacy-scope">${safe(categoryName(control.category))}${control.requiresAdmin ? ' · 需要管理员授权' : ''}</div>
    <p>${safe(control.description)}</p>
    <details><summary>范围、证据与恢复</summary><dl><dt>作用范围</dt><dd>${safe(control.scope || '尚未确认')}</dd><dt>本次检查</dt><dd>${safe(control.detail || '')} ${safe(readEvidence(control))}</dd><dt>可能的影响</dt><dd>${safe(control.impact || '以 Windows 当前版本和应用兼容性为准。')}</dd>${control.guide ? `<dt>边界与手动操作</dt><dd>${safe(control.guide)}</dd>` : ''}<dt>重新启动</dt><dd>${safe(restartText(control))}</dd><dt>恢复方式</dt><dd>只恢复本应用记录且仍由本应用管理的值。外部更改会提示冲突，不覆盖其他软件的新设置。</dd></dl></details>
    <div class="privacy-actions">${canApply(control) ? `<button class="btn sm primary" data-privacy-apply="${safe(control.id)}" ${model.busy || model.loading ? 'disabled' : ''}>${control.managed ? '重新应用并读回' : '应用并读回'}</button>` : ''}${control.managed ? `<button class="btn sm" data-privacy-restore="${safe(control.id)}" ${model.busy || model.loading || !model.reportReadable || (control.requiresAdmin && !helperReady()) ? 'disabled' : ''}>恢复原值</button>` : ''}${control.settingsId ? `<button class="btn sm" data-privacy-settings="${safe(control.settingsId)}">Windows 设置</button>` : ''}</div>${actionHint(control) ? `<p class="privacy-action-hint">${safe(actionHint(control))}</p>` : ''}
  </article>`;
    const systemPage = () => {
    const all = controls().filter(c => c.category !== 'browser' && c.category !== 'guidance');
    const categories = all.map(c => c.category || '系统设置').filter((c, i, a) => a.indexOf(c) === i);
    const matches = all.filter(c => (!model.category || (c.category || '系统设置') === model.category) && (!model.search || [c.title, c.description, c.detail, c.guide, c.scope].join(' ').toLowerCase().indexOf(model.search.toLowerCase()) >= 0));
    return `${timeCard()}<div id="privacy-region-slot"></div>${locationPortalCard()}<div class="privacy-filter"><input type="search" id="privacy-search" placeholder="搜索定位、遥测、广告等" value="${safe(model.search)}" aria-label="搜索保护项目"><select id="privacy-category" aria-label="保护类别"><option value="">所有类别</option>${categories.map(c => `<option value="${safe(c)}" ${model.category === c ? 'selected' : ''}>${safe(categoryName(c))}</option>`).join('')}</select></div><div id="privacy-controls">${matches.map(controlCard).join('') || `<p class="privacy-empty">${model.loading ? '正在读取本地设置…' : model.report ? '没有符合条件的项目。' : '点击“本地重新检查”读取可用项目。'}</p>`}</div>`;
  };
  function locationPortalCard(){
    const draft=model.portalDraft,r=model.portalReport,p=model.portalPreparation,disabled=model.portalBusy||model.busy||model.loading?'disabled':'',connected=!!(r&&r.connected);
    const names={not_connected:'尚未连接',connected:'已连接，未覆盖',active:'门户配置已确认',consumer_verified:'独立消费者匹配',external_override:'已有外部覆盖',external_conflict:'检测到外部更改',applying:'待恢复确认',restoring:'恢复待确认',restored:'覆盖已关闭',trajectory_stopped:'轨迹已停止'};
    const runtime=model.portalRuntime;
    const runtimeNote=runtime?`<div class="privacy-note ${runtime.status==='build_supported'?'':'warning'}" id="privacy-location-runtime"><strong>官方位置提供程序门禁：${safe(runtime.status==='build_supported'?'系统版本达到文档门槛':'当前版本不开放')}</strong><p>${safe(runtime.reason||'仅完成本地版本检查；不会启用位置提供程序。')}</p><p class="muted small">该检查不代表 Microsoft LAF、runFullTrust、会话所有权或原子写入已满足；Windows 10 / 11 仍使用下方实际可用的独立读取与 Device Portal 路径。</p></div>`:'<p class="privacy-note warning" id="privacy-location-runtime">尚未读取官方位置提供程序门禁；不会据此启用 GPS 写入。</p>';
    return `<article class="privacy-card" id="privacy-location-card"><div class="privacy-card-head"><h2>系统位置模拟</h2><span class="privacy-badge">${safe(r?names[r.status]||'状态待确认':'尚未检查')}</span></div><p>输入坐标或轨迹，通过 Windows 定位服务提供给使用该服务的应用。IP 和硬件 GPS 不变。</p>${runtimeNote}<div class="privacy-actions"><button class="btn sm primary" id="privacy-location-prepare" ${disabled}>准备并连接系统组件</button><button class="btn sm" id="privacy-location-read" ${disabled}>读取组件与覆盖状态</button>${p&&p.canRestore?`<button class="btn sm" id="privacy-location-unprepare" ${disabled}>恢复组件准备</button>`:''}</div>${p?`<p class="privacy-note" id="privacy-location-preparation">${safe(p.detail||p.status||'组件状态待确认')}</p>`:''}<details><summary>组件、权限与恢复范围</summary><p>首次准备可能需要管理员授权、安装微软开发组件并启用 Device Portal。启动前须确认仅本机可访问且认证开启；失败不会改为无认证或开放局域网。</p><p>Windows 的凭据初始化工具会短暂在其进程参数中使用本次随机门户密码。密码只在后端内存中保留，不保存到配置或日志。</p><p>先恢复位置覆盖，再恢复组件准备。门户没有原子外部比较替换；发现冲突会停止。恢复覆盖不代表清除系统注入位置缓存。</p></details><label for="privacy-location-label">位置标签（可选，仅本页）</label><input id="privacy-location-label" maxlength="96" value="${safe(draft.label)}" placeholder="给这个坐标取个名字，不进行在线地理编码" ${disabled}><div class="privacy-form"><label for="privacy-location-latitude">纬度（−90 至 90）</label><input id="privacy-location-latitude" inputmode="decimal" value="${safe(draft.latitude)}" placeholder="例如 31.2304" ${disabled}><label for="privacy-location-longitude">经度（−180 至 180）</label><input id="privacy-location-longitude" inputmode="decimal" value="${safe(draft.longitude)}" placeholder="例如 121.4737" ${disabled}></div><details><summary>按轨迹移动</summary><p>每行填写“纬度,经度”。最多 120 点、总时长 10 分钟；间隔最少 2 秒。填写不会执行，停止后最后位置仍需恢复。</p><textarea id="privacy-location-trajectory" rows="4" placeholder="31.2304,121.4737&#10;31.2310,121.4740" ${disabled}>${safe(draft.trajectory)}</textarea><label for="privacy-location-interval">每点间隔（秒）</label><input id="privacy-location-interval" inputmode="numeric" value="${safe(draft.intervalSeconds)}" ${disabled}></details><p id="privacy-location-draft-status" class="privacy-pending">${safe(model.portalMessage||'草稿未应用；不会自动修改系统位置。')}</p><p id="privacy-location-error" class="privacy-error" role="alert" ${model.portalError?'':'hidden'}>${safe(model.portalError)}</p><div class="privacy-actions"><button class="btn sm primary" id="privacy-location-apply" ${disabled||!connected||!r.canApply?'disabled':''}>应用坐标 / 轨迹</button><button class="btn sm" id="privacy-location-observe" ${disabled||!r||!r.configurationVerified||r.runningTrajectory?'disabled':''}>独立读取系统位置</button><button class="btn sm" id="privacy-location-restore" ${disabled||!r||!r.canRestore?'disabled':''}>恢复本应用覆盖</button>${r&&r.runningTrajectory?`<button class="btn sm" id="privacy-location-stop" ${disabled}>停止轨迹</button>`:''}</div>${r?`<dl id="privacy-location-configuration"><dt>门户覆盖</dt><dd>${r.override?'已开启':'未开启'}</dd><dt>配置核对</dt><dd>${r.configurationVerified?'门户当前坐标已读回；不是消费者证据':'尚未确认当前配置'}</dd>${r.configuredPosition?`<dt>门户当前坐标</dt><dd>${safe(r.configuredPosition.latitude)}, ${safe(r.configuredPosition.longitude)}</dd>`:''}${r.runningTrajectory?`<dt>轨迹进度</dt><dd>${safe(r.completedPoints)} / ${safe(r.totalPoints)}</dd>`:''}<dt>独立消费者</dt><dd>${r.consumerVerified?'新鲜 Default 来源与本次位置匹配；其他应用仍需验证':'尚未通过独立 Windows 定位读取（点击后显示独立授权窗口）'}</dd>${r.consumerVerified&&r.consumer?`<dt>消费者读回</dt><dd>${safe(r.consumer.position.latitude)}, ${safe(r.consumer.position.longitude)} · ${safe(r.consumer.timestamp)}</dd>`:''}</dl><p class="muted small">${safe(r.detail||'')}</p>`:''}<details><summary>连接已配置的本机门户</summary><p>只接受 HTTPS 127.0.0.1、证书 SHA-256 和认证。不会尝试外部 URL、明文连接或免认证回退。</p><label for="privacy-location-port">HTTPS 端口</label><input id="privacy-location-port" inputmode="numeric" value="${safe(draft.port)}" ${disabled}><label for="privacy-location-pin">证书 SHA-256（64 位小写十六进制）</label><input id="privacy-location-pin" value="${safe(draft.certificateSHA256)}" autocomplete="off" ${disabled}><label for="privacy-location-user">门户用户名</label><input id="privacy-location-user" value="${safe(draft.username)}" autocomplete="off" ${disabled}><label for="privacy-location-password">门户密码（仅本次内存）</label><input type="password" id="privacy-location-password" value="${safe(draft.password)}" autocomplete="new-password" ${disabled}><button class="btn sm" id="privacy-location-connect" ${disabled}>核验证书并连接</button></details><p class="muted small">关闭页面不会撤销已应用的位置；退出应用会停止轨迹，覆盖及私有恢复记录保留。请显式恢复。</p></article>`;
  }
  function portalRequest(){
    const draft=model.portalDraft,number=value=>{if(typeof value!=='string'||!/^[-+]?(?:\d+(?:\.\d*)?|\.\d+)$/.test(value.trim()))throw Error('请填写有效的十进制坐标。');return Number(value.trim());};
    const point=(latitude,longitude)=>{const p={latitude:number(latitude),longitude:number(longitude)};if(!Number.isFinite(p.latitude)||!Number.isFinite(p.longitude)||Math.abs(p.latitude)>90||Math.abs(p.longitude)>180)throw Error('纬度须在 −90 至 90，经度须在 −180 至 180。');return p;};
    let points,intervalSeconds=0;if(draft.trajectory.trim()){const lines=draft.trajectory.trim().split(/\r?\n/);if(lines.length>120)throw Error('轨迹最多 120 点。');points=lines.map(line=>{const pair=line.split(',');if(pair.length!==2)throw Error('轨迹每行须为“纬度,经度”。');return point(pair[0],pair[1]);});if(points.length>1){if(!/^\d+$/.test(draft.intervalSeconds))throw Error('轨迹间隔须为整数秒。');intervalSeconds=Number(draft.intervalSeconds);if(intervalSeconds<2||intervalSeconds>60||intervalSeconds*(points.length-1)>600)throw Error('轨迹间隔须为 2–60 秒，总时长不超过 10 分钟。');}}else points=[point(draft.latitude,draft.longitude)];return{points,intervalSeconds};
  }
  function bindLocationPortal(){
    const active=node=>alive()&&model.tab==='system'&&el.contains(node)&&!model.portalBusy&&!model.busy&&!model.loading&&!privacyWork.busy;
    const updateDraft=(id,key)=>{const input=el.querySelector('#'+id);if(input)input.addEventListener('input',()=>{if(!active(input))return;model.portalDraft[key]=input.value;model.portalGeneration++;model.portalError='';model.portalMessage='草稿已修改，尚未应用。';const message=el.querySelector('#privacy-location-draft-status'),error=el.querySelector('#privacy-location-error');if(message)message.textContent=model.portalMessage;if(error){error.hidden=true;error.textContent='';}});};
    for(const [id,key]of [['privacy-location-label','label'],['privacy-location-latitude','latitude'],['privacy-location-longitude','longitude'],['privacy-location-trajectory','trajectory'],['privacy-location-interval','intervalSeconds'],['privacy-location-port','port'],['privacy-location-pin','certificateSHA256'],['privacy-location-user','username'],['privacy-location-password','password']])updateDraft(id,key);
    const run=async(method,arg,preparation=false)=>{const generation=model.portalGeneration;model.portalBusy=true;if(method==='PrivacyLocationPortalObserve')model.portalObservationPending=true;model.portalError='';model.portalMessage='';draw();try{const result=await api(method,arg);if(!alive()||model.tab!=='system'||generation!==model.portalGeneration)return;if(!result||typeof result.status!=='string')throw Error('系统位置回执无法确认。');if(preparation){model.portalPreparation=result;model.portalMessage=result.detail||'组件操作已提交，以实际读回为准。';if(result.status==='restored')model.portalReport={status:'not_connected',connected:false,managed:false,canApply:false,canRestore:false,configurationVerified:false,consumerVerified:false};if(result.status==='ready'||result.ready||result.canConnect){const state=await api('PrivacyLocationPortalReport');if(alive()&&model.tab==='system'&&generation===model.portalGeneration)model.portalReport=state;}}else{model.portalReport=result;model.portalMessage=result.consumerVerified?'独立系统位置读取通过；其他应用仍需各自核对。':result.configurationVerified?'门户配置已读回；请再独立读取系统位置。':result.detail||'操作已返回，状态仍待确认。';if(method==='PrivacyLocationPortalConnect'&&result.connected){model.portalDraft.username='';model.portalDraft.password='';}}return result;}catch(error){if(alive()&&model.tab==='system'&&generation===model.portalGeneration){model.portalError='系统位置操作未完成：'+errText(error);model.portalMessage='草稿已保留；没有自动重试，请重新读取状态检查恢复。';if(model.portalReport)model.portalReport={...model.portalReport,status:'unknown',canApply:false,configurationVerified:false,consumerVerified:false,consumer:null};}}finally{if(method==='PrivacyLocationPortalObserve')model.portalObservationPending=false;model.portalBusy=false;if(alive())draw();}};
    const bind=(id,handler)=>{const button=el.querySelector('#'+id);if(button)button.addEventListener('click',()=>{if(active(button))handler(button);});};
    bind('privacy-location-read',async()=>{await run('PrivacyLocationPortalReport');});
    bind('privacy-location-connect',()=>run('PrivacyLocationPortalConnect',JSON.stringify({port:Number(model.portalDraft.port),certificateSHA256:model.portalDraft.certificateSHA256,username:model.portalDraft.username,password:model.portalDraft.password})));
    bind('privacy-location-apply',async button=>{let request;try{request=portalRequest();}catch(error){model.portalError=errText(error);draw();return;}const generation=model.portalGeneration;const ok=await context.confirm('将这 '+request.points.length+' 个位置应用到 Windows 定位服务？\n\n使用系统定位的其他应用可能受影响；不改变 IP。先保存恢复记录，应用后仍需独立读取验证。',{ok:'应用并读回'});if(ok&&active(button)&&generation===model.portalGeneration)await run('PrivacyLocationPortalApply',JSON.stringify(request));});
    bind('privacy-location-observe',()=>run('PrivacyLocationPortalObserve'));
    bind('privacy-location-stop',()=>run('PrivacyLocationPortalStopTrajectory'));
    bind('privacy-location-restore',async button=>{const generation=model.portalGeneration;if(await context.confirm('关闭本应用启用的位置覆盖？\n外部更改不会强制覆盖；不代表清除系统注入位置缓存。',{ok:'恢复覆盖'})&&active(button)&&generation===model.portalGeneration)await run('PrivacyLocationPortalRestore');});
    const poll=async generation=>{if(!alive()||model.tab!=='system'||generation!==model.portalPollGeneration||model.portalBusy)return;const r=await run('PrivacyLocationPortalPreparationReport',undefined,true);if(r&&['preparing','restoring'].includes(r.status)&&alive()&&generation===model.portalPollGeneration)setTimeout(()=>poll(generation),1500);};
    bind('privacy-location-prepare',async button=>{const generation=model.portalGeneration;if(await context.confirm('准备微软系统位置组件并建立仅本机认证连接？\n\n需要管理员助手；可能安装开发组件并启用相关系统功能。准备前保存原状态，禁止局域网访问与免认证；之后可单独恢复。',{ok:'准备系统组件'})&&active(button)&&generation===model.portalGeneration){const r=await run('PrivacyLocationPortalPrepare',undefined,true);if(r&&['preparing','restoring'].includes(r.status)){const own=++model.portalPollGeneration;setTimeout(()=>poll(own),1500);}}});
    bind('privacy-location-unprepare',async button=>{if(model.portalReport&&model.portalReport.managed&&model.portalReport.status!=='restored'){model.portalError='请先恢复本应用的位置覆盖，再恢复组件准备。';draw();return;}const generation=model.portalGeneration;if(await context.confirm('恢复本应用记录的组件准备？\n仅处理本应用的更改，遇到外部更改会保留并停止。',{ok:'恢复组件准备'})&&active(button)&&generation===model.portalGeneration)await run('PrivacyLocationPortalRestorePreparation',undefined,true);});
  }
  const firewallPage = () => {
    const fw = model.firewall || {};
    const rules = Array.isArray(fw.rules) ? fw.rules : [];
    const modeNames = { block_all: '安装直接连接限制（不含回环 / 代理）', block_lan: '限制此程序访问局域网', block_discovery: '限制组播和有限广播' };
    return `<p class="privacy-section-intro">按程序限制新建直接连接，添加后重新检查。回环、代理代发与原有连接另有边界。</p><details class="privacy-note warning"><summary>添加规则前了解影响</summary><p>限制浏览器、代理、内核或系统可能影响连接。程序规则独立于原有隧道，不能阻止其他已放行程序代发，也不会改变硬件信息。</p></details>
    <div class="privacy-card privacy-form"><h2>添加指定程序规则</h2><p class="muted">只针对用户选择的可执行文件。应用自身和连接所需进程由后端校验保护。默认不添加任何规则。</p><button class="btn sm" id="privacy-program" ${model.busy ? 'disabled' : ''}>选择 .exe 程序</button><div class="privacy-path">${safe(model.program || '尚未选择程序')}</div><label for="privacy-firewall-mode">阻止范围</label><select id="privacy-firewall-mode"><option value="block_lan">限制局域网访问</option><option value="block_discovery">限制组播和有限广播</option><option value="block_all">直接连接限制（不含回环 / 代理）</option></select><p class="privacy-pending">待应用：选择程序和范围后，点击确认添加才会修改规则。</p><div class="privacy-actions"><button class="btn sm primary" id="privacy-firewall-add" ${!model.program || model.busy || model.loading || !firewallReady() || duplicateRule() || fw.supported === false ? 'disabled' : ''}>确认添加规则</button></div></div>
    ${model.firewallError ? `<div class="privacy-error">${safe(model.firewallError)}</div>` : ''}${fw.scope ? `<div class="privacy-note">${safe(fw.scope)}${fw.loopback ? '<br>' + safe(fw.loopback === 'unprotected' ? '回环未保护：本机代理和转发路径不在此规则的保护范围内。' : fw.loopback) : ''}</div>` : ''}${fw.unknownFilters > 0 ? `<p class="privacy-note warning">还有 ${safe(fw.unknownFilters)} 条无法识别的过滤器，未视为保护；清理或卸载需要管理员处理。${privacyHelp('这些持久过滤器无法被安全识别或归属。规则列表为空也不代表没有遗留过滤器，不会把它们当作有效保护。')}</p>` : ''}${(fw.warnings || []).map(w => `<p class="privacy-note warning">${safe(w)}</p>`).join('')}
    ${rules.map(entry => { const rule = entry.rule || entry; return `<article class="privacy-card"><div class="privacy-card-head"><h2>${safe(modeNames[rule.mode] || rule.mode)}</h2><span class="privacy-badge">${safe(!model.firewallReadable ? '历史结果，当前未确认' : entry.state === 'partial' ? '过滤器不完整' : label({ status: entry.state }))}</span></div><p class="privacy-path">${safe(rule.program)}</p><p class="muted">${safe(entry.detail)}${entry.expectedFilters ? ' · 已安装过滤器 ' + safe(entry.installedFilters) + '/' + safe(entry.expectedFilters) : ''}</p><button class="btn sm" data-privacy-remove-rule="${safe(rule.id)}" ${model.busy || !firewallReady() ? 'disabled' : ''}>移除此规则</button></article>`; }).join('') || '<p class="privacy-empty">当前没有本应用记录的程序规则。</p>'}`;
  };
  const browserHostCard = () => {
    const host = model.host;
    const engines = host && Array.isArray(host.engines) ? host.engines : [];
    const badge = !host ? '尚未检查' : host.supported === false ? '当前平台不支持' : host.recommended ? 'WebView2 可用' : '未发现可用主机';
    return `${browserProfileCard()}${browserEnvironmentEditor()}${browserProxyCard()}${browserEngineCard()}<details class="privacy-card"><summary>浏览器运行时 · ${safe(badge)}</summary><p>这里只读取运行时清单。客户端界面、专用浏览器及现有 Chrome / Edge 配置分别管理。</p>${engines.length ? `<dl class="privacy-probe">${engines.map(engine => `<dt>${safe(engine.name)}${engine.available && engine.version ? ' · ' + safe(engine.version) : ''}</dt><dd>${safe(engine.available ? engine.detail : engine.detail || '未发现')}</dd>`).join('')}</dl>` : '<p class="muted">点击“本地重新检查”后查看。</p>'}</details>`;
  };
  const browserEngineRecord = () => {
    const id = model.browserEngine && model.browserEngine.engineId, version = model.browserEngine && model.browserEngine.version;
    return Array.isArray(model.engineVersions) ? model.engineVersions.find(item => item && item.engineId === id && item.version === version) : null;
  };
  const browserEngineReady = () => {
    if (!model.browser) return false;
    if (model.browserEngine.engineId === 'webview2') return model.browser.hostAvailable === true && model.browser.canLaunch === true;
    const item = browserEngineRecord();
    return !!(item && item.treeVerified === true && item.versionVerified === true && item.canLaunch === true);
  };
  const browserEngineManagerReady = () => {
    const item = Array.isArray(model.engineVersions) ? model.engineVersions.find(candidate => candidate && candidate.engineId === model.engineKind && candidate.version === model.engineVersion) : null;
    return !!(model.engineVersion && item && item.treeVerified === true && item.versionVerified === true && item.canLaunch === true);
  };
  const browserEngineCard=()=>{
    const catalog=model.engineCatalog,versions=Array.isArray(model.engineVersions)?model.engineVersions:[];
    const releases=catalog&&Array.isArray(catalog.releases)?catalog.releases.filter(r=>r.engineId===model.engineKind):[];
    const disabled=model.engineBusy||model.browserBusy?'disabled':'',deleteDisabled=browserEngineDeleteBlocked()?'disabled':'';
    return `<article class="privacy-card privacy-engine-manager"><div class="privacy-card-head"><h2>浏览器内核与版本${privacyHelp('从 Google Chrome for Testing 或 Mozilla 官方来源选择实际 Windows 版本。版本下载和完整性检查不等于已通过浏览器网络隔离验收；不会将任意路径或命令作为内核。')}</h2><span class="privacy-badge">${model.engineDeleting?'正在删除':model.engineBusy?'正在处理':'按需下载'}</span></div>
      <div class="privacy-browser-fields"><label>内核系列${privacyHelp('Chrome for Testing 是 Google 官方 Chrome 测试发行版；Firefox 使用实际 Gecko。两者均保留上游许可证。上方 WebView2 是另一个独立宿主。')}<select id="privacy-engine-kind" aria-label="下载浏览器内核" ${disabled}><option value="chrome-for-testing" ${model.engineKind==='chrome-for-testing'?'selected':''}>Google Chrome / Chromium</option><option value="firefox" ${model.engineKind==='firefox'?'selected':''}>Firefox / Gecko</option></select></label>
      <label>版本${privacyHelp('先刷新官方目录，然后选择或填写目录中的准确版本。多版本并存；不会覆盖已有目录或把 UA 版本当成真实内核版本。')}<input id="privacy-engine-version" aria-label="浏览器内核版本" list="privacy-engine-options" maxlength="40" value="${safe(model.engineVersion)}" placeholder="${catalog?'选择或输入目录中的准确版本':'先刷新官方目录'}" ${disabled}><datalist id="privacy-engine-options">${releases.slice(0,100).map(r=>`<option value="${safe(r.version)}">${safe(r.channel || r.version)}</option>`).join('')}</datalist></label>
      <label>下载连接${privacyHelp('读取目录和下载都只通过这里选择的路径；没有环境代理或直连回退。独立代理使用本页刚填写的地址和认证，不会保存密码。')}<select id="privacy-engine-route" aria-label="内核下载连接" ${disabled}><option value="ultra" ${model.engineRoute==='ultra'?'selected':''}>当前 Ultra 严格连接</option><option value="proxy" ${model.engineRoute==='proxy'?'selected':''}>本页独立代理</option></select></label></div>
      <div class="privacy-actions"><button class="btn sm" id="privacy-engine-refresh" ${disabled}>刷新官方版本</button><button class="btn sm primary" id="privacy-engine-download" ${disabled || !model.engineVersion?'disabled':''}>下载并校验此版本</button><button class="btn sm" id="privacy-engine-local" ${disabled}>检查本地版本</button><button class="btn sm" id="privacy-engine-launch" ${disabled || !browserEngineManagerReady()?'disabled':''}>启动此内核</button><button class="btn sm" id="privacy-engine-close" ${disabled}>关闭独立内核</button>${browserStartControls()}</div>
      <p class="muted">${catalog?'目录读取：'+safe(catalog.fetchedAt)+' · 当前系列 '+releases.length+' 个版本。可输入准确版本查找。':'不会自动请求官方目录。下载体积较大，需要时间解包并核对全部文件。'} 启动需要独立进程与代理隔离检查通过。</p>
      ${model.engineError?`<p class="privacy-error" role="alert">${safe(model.engineError)}</p>`:''}${model.engineMessage?`<p role="status">${safe(model.engineMessage)}</p>`:''}
      ${model.engineVersions?`<dl class="privacy-probe">${versions.map(v=>`<dt>${safe(v.engineId)} · ${safe(v.version)}</dt><dd>${v.treeVerified&&v.versionVerified?'文件与版本读回匹配':'完整性尚未确认'} · ${safe(v.fileCount)} 个文件${privacyHelp('仅为本地缓存读回；启动仍需重新建立可信执行、专用代理、进程生命周期和运行时配置证据。')} ${v.treeVerified&&v.versionVerified?`<button class="btn sm" data-browser-use-engine="${safe(v.engineId)}" data-browser-use-version="${safe(v.version)}" ${disabled}>使用此版本</button>`:''} <button type="button" class="btn sm" data-browser-delete-engine="${safe(v.engineId)}" data-browser-delete-version="${safe(v.version)}" aria-label="删除 ${safe(v.engineId)} ${safe(v.version)} 的内核文件" title="删除 ${safe(v.engineId)} ${safe(v.version)} 的内核安装文件" ${deleteDisabled}>删除</button></dd>`).join('') || '<dt>尚无已核验的本地版本</dt>'}</dl>`:''}</article>`;
  };
  const browserProxyCard = () => {
    const p = model.browserProxy, result = model.browserProxyResult, disabled = model.browserProxyBusy || model.browserBusy ? 'disabled' : '';
    const canLaunch=browserEngineReady();
    return `<article class="privacy-card privacy-browser-proxy"><div class="privacy-card-head"><h2>独立代理${privacyHelp('只用于本浏览器配置。不会修改系统代理或 VPN。连通性测试经填写的代理访问固定免费 API，失败不改用直连；测试成功也不等于浏览器实际出口已验证。')}</h2><span class="privacy-badge">${model.browserProxyBusy ? '正在测试' : result ? '代理探测通过' : '尚未测试'}</span></div>
      <div class="privacy-browser-fields"><label>浏览器连接${privacyHelp('此选择决定“打开浏览器”的连接路径。WebView2 保持 Ultra 严格连接；独立 Chrome / Firefox 当前使用下方代理。编辑和测试不会修改系统代理。')}<select id="privacy-browser-route" aria-label="浏览器连接方式" ${disabled}><option value="ultra" ${model.browserRoute==='ultra'?'selected':''} ${model.browserEngine.engineId!=='webview2'?'disabled':''}>当前 Ultra 严格连接</option><option value="proxy" ${model.browserRoute==='proxy'?'selected':''}>本页独立代理</option></select></label><label>代理类型${privacyHelp('SOCKS5 使用远端域名解析。HTTP 使用 CONNECT，HTTPS 还加密到代理服务器的链路；HTTP 代理认证在代理链路上不加密。')}<select id="privacy-proxy-type" aria-label="浏览器代理类型" ${disabled}>${['socks5','http','https'].map(type=>`<option value="${type}" ${p.type===type?'selected':''}>${type.toUpperCase()}</option>`).join('')}</select></label>
      <label>服务器${privacyHelp('只填写域名或 IP，不填写协议、路径或用户名。允许自己的本地代理；不自动猜测或替换服务器。')}<input id="privacy-proxy-host" aria-label="浏览器代理服务器" value="${safe(p.host)}" maxlength="253" placeholder="proxy.example.com" autocomplete="off" spellcheck="false" ${disabled}></label>
      <label>端口${privacyHelp('填写代理实际监听端口，范围 1–65535。')}<input id="privacy-proxy-port" aria-label="浏览器代理端口" type="number" min="1" max="65535" value="${safe(p.port)}" ${disabled}></label>
      <label>用户名（可选）${privacyHelp('留空表示无认证。认证只发给所选代理，不发给 IP 查询 API；不会加入诊断或命令行。')}<input id="privacy-proxy-username" aria-label="浏览器代理用户名" value="${safe(p.username)}" maxlength="255" autocomplete="off" spellcheck="false" ${disabled}></label>
      <label>密码（可选）${privacyHelp('密码仅保留在此页内存与本次代理请求中；离开页面后不保存。不使用 URL 携带密码。')}<input id="privacy-proxy-password" aria-label="浏览器代理密码" type="password" value="${safe(p.password)}" maxlength="255" autocomplete="new-password" ${disabled}></label></div>
      <p class="muted">点击测试才会经代理访问 country.is 免费 API；服务会看到该代理的出口 IP。归属地是 GeoIP 估计，与 GPS 无关。</p>
      <div class="privacy-actions"><button class="btn sm" id="privacy-proxy-test" ${disabled}>测试连通性与出口</button><button class="btn sm primary" id="privacy-proxy-launch" data-help-text="使用上方已经预览的配置和此代理打开专用浏览器。启动会重新验证连接、认证、进程与实际读回，不使用上次探测结果作为授权。" ${disabled || !canLaunch ? 'disabled' : ''}>用此代理打开浏览器</button></div><p class="muted">启动会重新核对受保护连接和代理绑定；测试成功也不替代浏览器实际导航出口验证。</p>
      ${model.browserProxyError ? `<p id="privacy-proxy-error" class="privacy-error" role="alert">${safe(model.browserProxyError)}</p>` : ''}${result ? `<div id="privacy-proxy-result"><dl class="privacy-probe"><dt>代理探测出口 IP</dt><dd>${safe(result.ip)}</dd><dt>归属地</dt><dd>${safe([result.country,result.subdivision,result.city].filter(Boolean).join(' / '))}</dd><dt>耗时 / 来源</dt><dd>${safe(result.latencyMs)} ms · ${safe(result.provider)} · ${safe(result.checkedAt)}</dd></dl><p class="muted">仅为此次代理探测结果，未证明浏览器实际使用此出口；本页不会将结果加入检测导出。</p></div>` : ''}</article>`;
  };
  const browserEnvironmentEditor = () => {
    const meta=model.browserMeta||{}, x=model.browserExtras||{}, storage=x.storage||{}, tabs=x.tabs||{}, proxy=x.proxy||{}, fp=x.fingerprint||{}, noise=fp.noise||{}, advanced=x.advanced||{}, readback=x.readback||{};
    const disabled=model.browserBusy||model.engineBusy||model.loading?'disabled':'';
    const options=(value,list)=>list.map(pair=>`<option value="${safe(pair[0])}" ${value===pair[0]?'selected':''}>${safe(pair[1])}</option>`).join('');
    const select=(path,label,value,list,help='')=>`<label>${safe(label)}${help?privacyHelp(help):''}<select data-browser-v2="${safe(path)}" ${disabled}>${options(value,list)}</select></label>`;
    const input=(path,label,value,attrs='',help='')=>`<label>${safe(label)}${help?privacyHelp(help):''}<input data-browser-v2="${safe(path)}" value="${safe(value)}" ${attrs} ${disabled}></label>`;
    const checked=(path,label,value,help='')=>`<label class="privacy-check"><input type="checkbox" data-browser-v2="${safe(path)}" ${value?'checked':''} ${disabled}>${safe(label)}${help?privacyHelp(help):''}</label>`;
    const status=(text,kind='pending')=>`<span class="privacy-capability ${kind}">${safe(text)}</span>`;
    const cookieCount=Number(storage.cookieCount)||0;
    const urlCount=Array.isArray(tabs.urls)?tabs.urls.length:0;
    const supported='运行时待验证';
    return `<article class="privacy-card privacy-browser-v2" id="privacy-browser-environment-editor">
      <div class="privacy-card-head"><h2>浏览器环境编辑器${privacyHelp('按 AdsPower / 比特浏览器式的完整环境配置组织。每个字段先保存为配置草稿，再在启动时按能力范围逐项读回；无法由当前宿主实现的项目保持“未知/不支持”，不会伪装成已生效。')}</h2>${status(readback.runtimeVerified?'运行时已读回':'配置已保存 · '+supported,readback.runtimeVerified?'verified':'pending')}</div>
      <p class="muted">配置名称是浏览器配置记录名称；环境名称、分组和备注用于管理。Cookie 原文只在本次会话内存中解析，默认不落盘。</p>
      <div class="privacy-browser-fields">
        ${select('meta.browser','浏览器',meta.browser||'sunbrowser',[['sunbrowser','SunBrowser'],['flowerbrowser','FlowerBrowser'],['chrome-for-testing','Chrome / Chromium'],['firefox','Firefox / Gecko'],['webview2','WebView2 / Edge']], '这里只保存浏览器家族标签；实际内核仍由下方浏览器内核和版本决定。')}
        ${select('meta.browser','浏览器家族',meta.browser||'chromium',[['sunbrowser','SunBrowser（家族标签）'],['flowerbrowser','FlowerBrowser（家族标签）'],['chromium','Chrome / Chromium'],['firefox','Firefox / Gecko'],['webview2','WebView2 / Edge']], '家族标签与实际内核分开保存；选择 SunBrowser 或 FlowerBrowser 不会伪装成未安装的真实内核。')}
        ${input('meta.environmentName','环境名称',meta.environmentName||'','maxlength="100" placeholder="选填：环境名称"','环境名称最多 100 个字符，只用于识别配置。')}
        ${input('meta.group','分组',meta.group||'','maxlength="100" placeholder="未分组"','分组不会改变启动参数。')}
        ${input('meta.tags','标签（逗号分隔）',Array.isArray(meta.tags)?meta.tags.join(', '):'','maxlength="512" placeholder="例如：工作, 社媒"','标签最多 32 个，每个标签最多 64 个字符。')}
        <label>备注${privacyHelp('备注最多 2000 个字符，只保存文本，不会发送到代理或站点。')}<textarea data-browser-v2="meta.remark" maxlength="2000" rows="3" placeholder="请输入备注" ${disabled}>${safe(meta.remark||'')}</textarea></label>
      </div>
      <details open><summary>Cookie 与启动标签页 ${status(cookieCount+' 个 Cookie · 会话内存')}${status(urlCount+' 个启动网址','configured')}</summary>
        <div class="privacy-browser-fields">
          ${select('storage.cookieFormat','Cookie 格式',storage.cookieFormat||'json',[['json','JSON'],['netscape','Netscape'],['name-value','Name=Value']], '支持 JSON、Netscape、Name=Value；只解析格式和数量。')}
          ${checked('storage.merge','合并 Cookie',!!storage.merge,'开启后导入的 Cookie 与现有会话合并；冲突以当前会话策略为准。')}
        </div>
        <label>Cookie（本次会话，不落盘）${privacyHelp('粘贴后立即在本地解析并显示数量；关闭页面或切换配置后清空。保存配置只保存格式、合并策略和数量，不保存 Cookie 值。')}<textarea data-browser-v2-cookie rows="5" placeholder='JSON、Netscape 或 name=value' ${disabled}>${safe(model.browserCookieDraft||'')}</textarea></label>
        <p class="muted">${storage.cookieSessionOnly!==false?'安全策略：Cookie 原文仅保留在本页内存。':''}解析结果：${safe(cookieCount)} 个。</p>
        <div class="privacy-browser-fields">
          <label>启动网址（每行一个）${privacyHelp('最多 20 个 HTTPS/HTTP URL；保存前会校验协议和长度。')}<textarea data-browser-v2="tabs.urls" rows="4" placeholder="www.google.com&#10;www.facebook.com" ${disabled}>${safe(Array.isArray(tabs.urls)?tabs.urls.join('\n'):'')}</textarea></label>
          ${select('tabs.startupMode','标签页启动方式',tabs.startupMode||'restore',[['restore','恢复上次标签页'],['urls','打开指定网址'],['blank','空白页']], '指定网址只在启动时使用，不会自动访问或预加载。')}
        </div>
      </details>
      <details open><summary>代理信息 ${status(proxy.mode==='no-proxy'?'No Proxy':proxy.mode==='custom'?'自定义':proxy.mode==='saved'?'已添加的代理':proxy.mode==='dynamic'?'动态代理':'集成代理')}</summary>
        <div class="privacy-browser-fields">
          ${select('proxy.mode','代理方式',proxy.mode||'no-proxy',[['no-proxy','No Proxy（本地直连）'],['custom','自定义'],['saved','已添加的代理'],['dynamic','动态代理'],['integrated','集成代理']], '代理来源只记录选择，不会把密码写入浏览器配置。')}
          ${select('proxy.type','代理类型',proxy.type||'none',[['none','No Proxy'],['http','HTTP'],['https','HTTPS'],['socks5','SOCKS5']], '当前页面的独立代理测试仍使用下方代理卡；这里保存环境的来源策略。')}
          ${input('proxy.provider','IP 查询渠道',proxy.provider||'IP2Location','maxlength="80" placeholder="IP2Location"','只记录查询渠道名称，查询结果不代表 GPS 或浏览器真实出口。')}
          ${input('proxy.accountRef','账号平台',proxy.accountRef||'','maxlength="120" placeholder="账号平台 / 账号引用"','只保存账号引用，不保存账号密码。')}
        </div>
        <p class="muted">网络检查仍需点击“独立代理”卡片中的测试按钮；配置保存不会宣称代理已连通。</p>
      </details>
      <details open><summary>指纹配置 ${status('字段已保存，启动后读回')}</summary>
        <div class="privacy-browser-fields">
          ${select('fingerprint.webrtc','WebRTC',fp.webrtc||'disable-non-proxied-udp',[['forward','转发'],['replace','替换'],['real','真实'],['disabled','禁用'],['disable-non-proxied-udp','代理 UDP / 禁止非代理 UDP']], 'WebRTC 由浏览器内核和扩展共同决定；当前宿主仅对既有非代理 UDP 限制有运行时路径。')}
          ${select('fingerprint.timezone','时区',fp.timezone||'real',[['ip','基于 IP'],['real','真实'],['custom','自定义']], 'IP 方案需要可信代理出口；本页面不会自动猜测时区。')}
          ${select('fingerprint.geo','地理位置',fp.geo||'real',[['ip','基于 IP'],['custom','自定义'],['forbidden','禁止']], '网页定位需要单独的浏览器权限与系统位置提供者；无法读回时保持未知。')}
          ${select('fingerprint.language','语言',fp.language||'real',[['ip','基于 IP'],['custom','自定义'],['real','真实']], '语言策略与请求头、navigator.languages 需在启动后一起核验。')}
          ${select('fingerprint.uiLanguage','界面语言',fp.uiLanguage||'by-language',[['by-language','基于语言'],['real','真实'],['custom','自定义']], '只保存策略，不修改 Ultra 客户端界面语言。')}
          ${select('fingerprint.resolution','分辨率',fp.resolution||'ua',[['random','随机'],['preset','预定义'],['custom','自定义'],['ua','基于 User-Agent']], '视口和 DPR 由现有运行时部分支持；物理显示器不会被修改。')}
          ${select('fingerprint.fonts','字体',fp.fonts||'default',[['default','默认'],['custom','自定义']], '字体列表需要宿主级注入；当前仅保存计划并在启动后报告能力。')}
        </div>
        <div class="privacy-v2-noise"><strong>硬件噪音</strong>${checked('fingerprint.noise.canvas','Canvas',!!noise.canvas)}${checked('fingerprint.noise.webglImage','WebGL 图像',!!noise.webglImage)}${checked('fingerprint.noise.audio','AudioContext',!!noise.audio)}${checked('fingerprint.noise.media','媒体设备',!!noise.media)}${checked('fingerprint.noise.clientRects','ClientRects',!!noise.clientRects)}${checked('fingerprint.noise.speechVoices','SpeechVoices',!!noise.speechVoices)}</div>
        <div class="privacy-browser-fields">
          ${select('fingerprint.webglMetadata','WebGL 元数据',fp.webglMetadata||'real',[['real','真实'],['custom','自定义']], '厂商和渲染器可在下方编辑；真实硬件、像素和性能不会改变。')}
          ${input('fingerprint.webglVendor','厂商',fp.webglVendor||'Google Inc. (AMD)','maxlength="128" placeholder="Google Inc. (AMD)"')}
          ${input('fingerprint.webglRenderer','渲染器',fp.webglRenderer||'ANGLE (AMD, Radeon RX 580 Series (0x000067DF) Direct3D11 vs_5_0 ps_5_0, D3D11)','maxlength="256" placeholder="ANGLE (...)"')}
          ${select('fingerprint.webgpu','WebGPU',fp.webgpu||'runtime',[['webgl','基于 WebGL'],['real','真实'],['disabled','禁用']], 'WebGPU 的真实/禁用状态必须由运行时读回；名称策略不能替代能力检查。')}
        </div>
      </details>
      <details><summary>高级设置 ${status('真实硬件项可能仅支持读回','limited')}</summary>
        <div class="privacy-browser-fields">
          ${select('advanced.cpu','CPU',advanced.cpu||'real',[['real','真实'],['custom','自定义']], '当前运行时支持网页逻辑处理器数的有限覆盖；物理 CPU 不变。')}
          ${select('advanced.ram','RAM',advanced.ram||'real',[['real','真实'],['custom','自定义']], '真实 RAM 不能由普通网页或浏览器配置安全修改。')}
          ${select('advanced.deviceName','设备名称',advanced.deviceName||'real',[['real','真实'],['custom','自定义']], '设备名的系统级读回依赖宿主能力。')}
          ${select('advanced.mac','MAC 地址',advanced.mac||'real',[['real','真实'],['custom','自定义']], 'MAC 地址属于系统网络适配器，普通浏览器不能修改。')}
          ${select('advanced.dnt','Do Not Track',advanced.dnt||'default',[['default','默认'],['on','开启'],['off','关闭']], '启动后核对请求头和 navigator.doNotTrack。')}
          ${select('advanced.portScan','端口扫描保护',advanced.portScan||'enabled',[['enabled','启用'],['disabled','关闭']], '浏览器策略只能限制部分网页探测；不能替代系统防火墙。')}
          ${select('advanced.hardwareAcceleration','硬件加速',advanced.hardwareAcceleration||'default',[['default','默认'],['enabled','开启'],['disabled','关闭']], '需重启浏览器，并且可能受内核策略覆盖。')}
          ${select('advanced.tls','禁用 TLS 特性',advanced.tls||'default',[['default','默认'],['enabled','开启'],['disabled','关闭']], 'TLS 参数必须由实际内核支持；当前仅记录计划并读回。')}
          ${select('advanced.teamApps','团队应用',advanced.teamApps||'global',[['global','跟随团队应用'],['custom','自定义'],['disabled','禁用']], '应用安装需要团队应用中心，当前配置只记录策略。')}
          ${select('advanced.dataSync','数据同步',advanced.dataSync||'global',[['global','跟随全局设置'],['custom','自定义'],['disabled','关闭']], '只记录同步策略，不上传本地配置。')}
          ${select('advanced.browserSettings','浏览器设置',advanced.browserSettings||'global',[['global','跟随全局设置'],['custom','自定义']], '浏览器设置属于独立内核 profile。')}
          ${checked('advanced.randomFingerprint','随机指纹（每次启动生成）',!!advanced.randomFingerprint,'生成新 seed 后必须重新读回；不代表匿名或稳定身份。')}
        </div>
        <label>允许被扫描的端口（选填）${privacyHelp('只记录端口清单，最多 32 个；实际拦截范围取决于浏览器和系统。')}<input data-browser-v2="advanced.allowedPorts" value="${safe(Array.isArray(advanced.allowedPorts)?advanced.allowedPorts.join(', '):'')}" maxlength="256" placeholder="例如 80, 443" ${disabled}></label>
      </details>
      <div class="privacy-v2-summary"><div><strong>概要</strong><dl><dt>环境</dt><dd>${safe(meta.environmentName||'未命名环境')}</dd><dt>浏览器</dt><dd>${safe(meta.browser||model.browserEngine.engineId||'未选择')} · ${safe(model.browserProfileName||'未命名配置')}</dd><dt>标签页 / Cookie</dt><dd>${safe(urlCount)} / ${safe(cookieCount)}</dd><dt>代理</dt><dd>${safe(proxy.mode||'no-proxy')} · ${safe(proxy.type||'none')}</dd><dt>读回</dt><dd>${safe(readback.state||'unknown')} · ${safe(readback.runtimeVerified?'已验证':'未运行时读回')}</dd></dl></div><div class="privacy-actions"><button type="button" class="btn sm" id="privacy-browser-v2-generate" ${disabled}>生成新指纹</button><button type="button" class="btn sm" id="privacy-browser-v2-validate" ${disabled}>检查配置</button></div></div>
      ${model.browserV2Error?`<p class="privacy-error" role="alert">${safe(model.browserV2Error)}</p>`:''}${model.browserV2Message?`<p class="privacy-note" role="status">${safe(model.browserV2Message)}</p>`:''}
    </article>`;
  };
  const browserProfileCard = () => {
    const info = model.browser, c = info && info.config;
    if (!c) return `<article class="privacy-card"><h2>专用浏览器${privacyHelp('使用独立窗口和浏览数据。需要完整安装的浏览器组件；不会在客户端管理界面加载外部网页。')}</h2><p>${safe(model.browserError || '当前安装未提供专用浏览器组件。')}</p></article>`;
    const disabled = model.browserBusy || model.engineBusy || model.loading ? 'disabled' : '';
    const regions = Array.isArray(info.regions) ? info.regions : [];
    const environments = Array.isArray(info.environmentPresets) ? info.environmentPresets : [];
    const environment = c.environment && c.environment.mode === 'browser-emulation' ? c.environment : null;
    const mobile = environment && ['android','ios'].includes(environment.os);
    const environmentNames = {windows:'Windows',macos:'macOS',android:'Android',ios:'iOS'};
    const graphics = c.graphics && c.graphics.mode === 'webgl-metadata' ? c.graphics : null;
    const graphicsSupported = model.browserEngine.engineId !== 'chrome-for-testing';
    const graphicsCapability = model.browserEngine.engineId === 'firefox' ? 'Firefox：使用 Gecko 原生偏好覆盖 WebGL 名称并阻断 WebGPU；Window / Offscreen 启动读回。Worker 执行保持封锁，没有 Worker API 运行读回；真实 VM 效果待验。' : model.browserEngine.engineId === 'chrome-for-testing' ? 'Chrome 当前仅支持原生 GPU / WebGPU；此宿主尚未接可靠的原生名称覆盖。已有自定义草稿保留，请恢复原生图形信息或选择 Firefox / WebView2。' : 'WebView2：仅通过 JS 层覆盖 WebGL 名称，可检测，不改变硬件、图像或性能；WebGPU 使用原生阻断。跨源效果仍待实机验证。';
    const graphicsPresets = (!environment || environment.os === 'windows') && Array.isArray(info.graphicsPresets) ? info.graphicsPresets : [];
    const graphicsChoice = graphics ? graphics.presetId : 'native';
    const identityMode=c.identity && c.identity.mode || 'generated';
    const identityPreview=model.browserIdentityPreview;
    const uaValue=identityMode==='manual' ? c.identity.userAgent || '' : identityPreview && identityPreview.userAgent || '';
    const uaPlaceholder=identityPreview && identityPreview.userAgentTemplate || (identityMode==='native' ? '启动时读取真实浏览器 UA' : '正在读取本地内核信息并生成 UA');
    const localVersions=(Array.isArray(model.engineVersions)?model.engineVersions:[]).filter(v=>v.engineId===model.browserEngine.engineId&&v.treeVerified&&v.versionVerified);
    const pendingVersion=model.browserEngine.version&&!localVersions.some(v=>v.version===model.browserEngine.version);
    const selectedVersion = browserEngineRecord();
    const ready = browserEngineReady();
    const exitCleanup = c.exitCleanup === 'delete-owned-profile' ? 'delete-owned-profile' : 'retain-profile';
    const launchLabel = ready ? (model.browserPreview ? '打开浏览器' : '检查并打开') : model.browserEngine.engineId === 'webview2' ? (info.hostAvailable ? '检查并打开' : '需完整安装') : selectedVersion ? '检查并打开' : '请选择已核验内核';
    return `<article class="privacy-card privacy-browser-profile"><div class="privacy-card-head"><h2>专用浏览器${privacyHelp('独立保存浏览数据；只在已确认的受保护隧道内打开。隧道或控制连接失效时停止浏览，不改你的 VPN 设置。')}</h2><span class="privacy-badge">${!info.hostAvailable ? '需要完整安装' : info.canLaunch === true ? '启动路径已开放' : '组件已核验，待启动验收'}</span></div>
      <p>先选配置并预览，再打开浏览器。</p><div class="privacy-browser-fields">
      <label>浏览器内核${privacyHelp('选择实际内核。Chrome / Firefox 可从“内核版本”下载多个官方版本；选中后 UA、环境检查和启动都使用此版本。网页系统环境模拟不替换实际内核。')}<select id="privacy-browser-engine" aria-label="专用浏览器内核" ${disabled}><option value="webview2" ${model.browserEngine.engineId==='webview2'?'selected':''}>WebView2 / Edge</option><option value="chrome-for-testing" ${model.browserEngine.engineId==='chrome-for-testing'?'selected':''}>Google Chrome / Chromium</option><option value="firefox" ${model.browserEngine.engineId==='firefox'?'selected':''}>Firefox / Gecko</option></select></label>
      <label>内核版本${privacyHelp('列出已下载并核对的版本；载入的历史版本会标记缓存待检查。安装新版本请切换“内核版本”；不会把 UA 数字当成已安装内核。启动时重新核对。')}<select id="privacy-browser-engine-version" aria-label="使用的浏览器内核版本" ${disabled || model.browserEngine.engineId==='webview2'?'disabled':''}>${model.browserEngine.engineId==='webview2'?'<option value="">使用本机 WebView2 运行时</option>':'<option value="">请选择已下载版本</option>'+(pendingVersion?`<option value="${safe(model.browserEngine.version)}" selected>${safe(model.browserEngine.version)}（本地缓存待检查）</option>`:'')+localVersions.map(v=>`<option value="${safe(v.version)}" ${v.version===model.browserEngine.version?'selected':''}>${safe(v.version)}</option>`).join('')}</select></label>
      <label>配置名称${privacyHelp('可填写中文，保存时生成独立标识。只保存本页环境、UA、图形和内核选择；不保存代理地址、认证、订阅或启动授权。')}<input id="privacy-browser-profile-name" aria-label="浏览器配置名称" maxlength="80" value="${safe(model.browserProfileName)}" placeholder="例如：日常浏览" ${disabled}></label>
      <label>退出时数据${privacyHelp('选择关闭专用会话后的数据处理方式。删除只针对本次会话拥有的数据树，失败会保留并报告；共享内核缓存和其他配置不会删除。')}<select id="privacy-browser-exit-cleanup" aria-label="浏览器退出时数据处理" ${disabled}><option value="retain-profile" ${exitCleanup==='retain-profile'?'selected':''}>保留浏览数据</option><option value="delete-owned-profile" ${exitCleanup==='delete-owned-profile'?'selected':''}>删除本会话数据</option></select></label>
      <label>语言与时区${privacyHelp('成组选取语言、地区和时区，避免相互矛盾。仅影响专用浏览器，不改系统设置。')}<select id="privacy-browser-region" aria-label="浏览器语言与时区" ${disabled}>${regions.map(r => `<option value="${safe(r.presetId)}" ${r.presetId === c.region.presetId ? 'selected' : ''}>${safe(r.language)} · ${safe(r.timezone)}</option>`).join('')}</select></label>
      <label>系统环境${privacyHelp('选择网页可见的系统、设备、视口与触控模板，之后可手动编辑。实际运行所选 Windows 浏览器内核；iOS 是合成环境，UA 保留真实 Blink/Gecko 标识，不是零售 iOS 浏览器或 Safari，也不改变全部字体、渲染与硬件特征。')}<select id="privacy-browser-environment" aria-label="浏览器系统环境" ${disabled}><option value="native" ${!environment ? 'selected' : ''}>使用本机浏览器环境</option>${environments.map(p=>`<option value="${safe(p.config.devicePresetId)}" ${environment && environment.devicePresetId===p.config.devicePresetId?'selected':''}>${safe(environmentNames[p.config.os] || p.config.os)} · ${safe(p.model || '桌面')}</option>`).join('')}</select></label>
      <label>${environment&&environment.os==='windows'?'平台版本':'系统版本'}${privacyHelp(environment&&environment.os==='windows'?'这是 Windows Client Hints 的 API 合约平台版本，不是营销版号。快捷预设为 Windows 10 → 10.0.0、Windows 11 → 13.0.0；可以继续手动编辑。现代生成 UA 均保留 Windows NT 10.0。':'输入 1–3 段数字，例如 16.0。浏览器版本来自实际内核；现代 UA 可能使用规范冻结的 OS 字符串，详细环境参数独立协调。iOS 为合成环境。')}<input id="privacy-browser-os-version" aria-label="浏览器环境系统版本" value="${safe(environment ? environment.osVersion : '')}" maxlength="11" inputmode="decimal" placeholder="使用原生信息" ${disabled || !environment ? 'disabled' : ''}></label>
      <label>UA 方式${privacyHelp('自动模式根据环境与真实内核生成规范 UA；手动模式逐字使用输入；原生模式保留实际 UA 文本。所选网页环境仍独立覆盖平台、CH、视口等，原生/手动 UA 可能与环境不同。启动时读回，预览不是应用。')}<select id="privacy-browser-identity-mode" aria-label="User-Agent方式" ${disabled}><option value="generated" ${identityMode==='generated'?'selected':''}>根据环境生成</option><option value="manual" ${identityMode==='manual'?'selected':''}>手动填写</option><option value="native" ${identityMode==='native'?'selected':''}>保留原生 UA</option></select></label>
      <label>User-Agent${privacyHelp('可直接修改；修改会切换到手动模式并保留原文。这里只接受可打印 ASCII，最多 1024 字符；启动后还会核对 HTTP 请求与网页读取值。预览不代表已经应用。')}<textarea id="privacy-browser-ua" aria-label="浏览器User-Agent" maxlength="1024" rows="3" spellcheck="false" placeholder="${safe(uaPlaceholder)}" ${disabled}>${safe(uaValue)}</textarea></label>
      <div class="privacy-ua-actions"><button type="button" class="btn sm" id="privacy-browser-ua-generate" data-help-text="按当前系统环境重新生成 UA，替换手动输入；不启动浏览器、不请求网络。" ${disabled}>重新生成 UA</button><span id="privacy-browser-ua-status" role="status">${identityMode==='manual'?'将使用手动填写的 UA':identityPreview && identityPreview.exactChromiumVersionKnown?'来自当前运行时身份；启动时重新核对':'本地生成预览；启动时核对真实内核'}</span></div>
      <label>窗口宽度${privacyHelp('设置浏览内容区域的宽度。移动模板同时应用实际视口、DPR 和触控覆盖；不更换物理显示器。')}<input id="privacy-browser-width" aria-label="浏览器窗口宽度" type="number" min="${mobile ? 320 : 800}" max="3840" value="${safe(c.viewport.width)}" ${disabled}></label>
      <label>窗口高度${privacyHelp('设置浏览内容区域的高度。过小或过大的值会被拒绝。')}<input id="privacy-browser-height" aria-label="浏览器窗口高度" type="number" min="${mobile ? 568 : 600}" max="2160" value="${safe(c.viewport.height)}" ${disabled}></label>
      <label>随机种子${privacyHelp('相同种子和配置名生成相同组合。已选择自定义 GPU 时也从成对模板选取；原生模式不自动改 GPU。随机化不等于匿名，生成后仍需预览。')}<input id="privacy-browser-seed" aria-label="浏览器随机种子" maxlength="64" value="${safe(model.browserSeed)}" placeholder="填写后点击生成" ${disabled}></label></div>
      <h3 class="privacy-data-subtitle">GPU 与图形信息${privacyHelp('设置专用浏览器网页可见的 WebGL 厂商和渲染器名称。脚本层覆盖可被识别；不改变真实显卡、驱动、能力、渲染像素或性能。其他应用不受影响。')}</h3>
      <div class="privacy-browser-fields">
      <p id="privacy-browser-graphics-capability" class="muted">${safe(graphicsCapability)}</p>
      <label>GPU 预设${privacyHelp('预设是成对的网页 WebGL 可见字符串，不代表真实硬件或 Firefox / Chrome 内核身份。自定义必须阻断 WebGPU，启动核验失败停止。')}<select id="privacy-browser-graphics" aria-label="浏览器GPU预设" ${disabled}><option value="native" ${graphicsChoice === 'native' ? 'selected' : ''}>使用原生信息</option>${graphicsPresets.map(g => `<option value="${safe(g.presetId)}" ${graphicsChoice === g.presetId ? 'selected' : ''} ${!graphicsSupported?'disabled':''}>${safe(g.renderer)}</option>`).join('')}<option value="manual" ${graphicsChoice === 'manual' ? 'selected' : ''} ${!graphicsSupported?'disabled':''}>手动填写厂商与设备名称</option></select></label>
      <label>WebGPU${privacyHelp('与 WebGL 分开控制。Firefox 使用原生开关，要求安全 Window 中接口不存在；WebView2 原生阻断适配器且接口可能仍可见。仅名称改变不能证明 GPU 指纹隐藏。')}<select id="privacy-browser-webgpu" aria-label="浏览器WebGPU模式" ${disabled || graphics ? 'disabled' : ''}><option value="runtime-default" ${c.webgpu !== 'disabled' ? 'selected' : ''}>使用原生适配器</option><option value="disabled" ${c.webgpu === 'disabled' ? 'selected' : ''} ${!graphicsSupported?'disabled':''}>阻断原生适配器</option></select></label>
      <label>WebGL 厂商${privacyHelp('网页通过 UNMASKED_VENDOR_WEBGL 读取的名称。限可打印 ASCII，最多 128 字符；不填写脚本、命令或路径。')}<input id="privacy-browser-gpu-vendor" aria-label="浏览器WebGL厂商" maxlength="128" value="${safe(graphics ? graphics.vendor : '')}" placeholder="原生值在独立浏览器中读取" ${disabled || !graphics || !graphicsSupported ? 'disabled' : ''}></label>
      <label>GPU 设备 / 渲染器名称${privacyHelp('网页通过 UNMASKED_RENDERER_WEBGL 读取的名称，最多 256 个可打印 ASCII。Firefox 普通 VENDOR / RENDERER 仍按 Gecko 自身规则返回，可能与此不同；不修改设备管理器或物理硬件。')}<input id="privacy-browser-gpu-renderer" aria-label="浏览器GPU设备名称" maxlength="256" value="${safe(graphics ? graphics.renderer : '')}" placeholder="例如 ANGLE (...)" ${disabled || !graphics || !graphicsSupported ? 'disabled' : ''}></label>
      <label>CPU 核数${privacyHelp('设置网页读取的逻辑处理器数。WebView2 / Chrome 使用真实内核调试接口并在启动读回，不改变物理 CPU 或运行速度。Firefox 当前保留原生值；切换内核不会悄悄丢弃已填参数。')}<select id="privacy-browser-cpu-mode" aria-label="网页CPU核数模式" ${disabled}><option value="native" ${!c.hardware?'selected':''}>使用原生信息</option><option value="logical-processors" ${c.hardware?'selected':''} ${model.browserEngine.engineId==='firefox'?'disabled':''}>自定义网页核数</option></select></label>
      <label>逻辑处理器数${privacyHelp('填写 1–64 的整数，网页 navigator.hardwareConcurrency 必须读回一致才允许浏览；浏览器不支持时会停止启动。')}<input id="privacy-browser-cpu" aria-label="网页逻辑处理器数" type="number" min="1" max="64" step="1" value="${safe(c.hardware?c.hardware.logicalProcessors:'')}" placeholder="原生核数在浏览器中读取" ${disabled || !c.hardware?'disabled':''}></label></div>
      <p class="muted">${graphics ? '自定义 WebGL 名称；WebGPU 适配器阻断。打开时检查 WebGL1/2 和图形配置，未匹配即停止。' : 'GPU 保留原生信息。可选择预设或手动编辑网页可见名称。'}</p>
      <p class="muted">UA 使用真实内核信息；定位、相机、麦克风、通知和剪贴板读取均拒绝。WebRTC 限制非代理 UDP。</p>
      <div class="privacy-profile-library"><div class="privacy-actions"><button type="button" class="btn sm" id="privacy-browser-profile-save" ${disabled}>${model.browserSavedRecord?'保存修改':'保存配置'}</button><button type="button" class="btn sm" id="privacy-browser-profile-list" ${disabled}>读取已保存配置</button>${model.browserSavedRecord?`<button type="button" class="btn sm" id="privacy-browser-profile-new" ${disabled}>另存为新配置</button><button type="button" class="btn sm" id="privacy-browser-profile-delete" ${disabled}>删除此配置记录</button>`:''}</div>${model.browserSavedList?`<div class="privacy-inline-controls"><select id="privacy-browser-profile-select" aria-label="已保存的浏览器配置" ${disabled}><option value="">选择配置</option>${model.browserSavedList.map(r=>`<option value="${safe(r.id)}" ${r.id===model.browserSavedSelection?'selected':''}>${safe(r.name)} · ${safe(r.engine.engineId)}${r.recoveryNeeded?' · 需恢复':''}</option>`).join('')}</select><button type="button" class="btn sm" id="privacy-browser-profile-load" ${disabled || !model.browserSavedSelection?'disabled':''}>载入</button></div>`:''}<p class="muted" id="privacy-browser-profile-status">${browserSavedProfileStatus()}</p><details><summary>配置标识</summary><input id="privacy-browser-id" aria-label="浏览器内部配置标识" value="${safe(c.profileId)}" maxlength="48" ${disabled}><p class="muted">保存后由系统生成独立标识，不使用名称作为文件路径。</p></details></div>
      ${!info.hostAvailable || info.canLaunch !== true ? `<p class="privacy-note">${safe(info.detail || '当前环境尚未满足独立浏览器启动条件；请先完成组件、版本、连接和进程隔离检查。')}</p>` : ''}
      ${model.browserError ? `<p id="privacy-browser-error" class="privacy-error" role="alert">${safe(model.browserError)}</p>` : ''}${model.browserMessage ? `<p id="privacy-browser-message" role="status">${safe(model.browserMessage)}</p>` : ''}
      ${model.browserEngine.engineId!=='webview2'?`<div class="privacy-native-helper"><button type="button" class="btn sm" id="privacy-browser-authorize" data-help-text="为本次独立内核的进程网络隔离授权固定管理员助手；浏览器窗口保持普通用户权限。取消后不启动，也不改变 VPN。" ${disabled || model.helper&&model.helper.state==='connected'?'disabled':''}>${model.helper&&model.helper.state==='connected' ? (model.helper.browserGuard && model.helper.browserGuard.available===false ? '浏览器隔离未就绪' : '基础助手已连接') : '授权网络隔离助手'}</button><span class="muted">${model.helper&&model.helper.state==='connected' ? (model.helper.browserGuard && model.helper.browserGuard.available===false ? '浏览器隔离尚未就绪，请查看系统权限中的阶段码。' : '基础助手已连接，启动时另核对浏览器隔离。') : '此内核需要助手；点击授权后按 Windows 提示操作。'}</span></div>`:''}
      <div class="privacy-actions"><button class="btn sm" id="privacy-browser-random" ${disabled}>随机生成</button><button class="btn sm" id="privacy-browser-preview" ${disabled}>检查配置</button><button class="btn sm primary" id="privacy-browser-launch" ${disabled || !ready ? 'disabled' : ''}>${launchLabel}</button><button class="btn sm" id="privacy-browser-close" ${disabled || !ready ? 'disabled' : ''}>关闭浏览器</button>${browserStartControls()}</div>
      ${model.browserPreview ? `<div id="privacy-browser-preview-result"><dl class="privacy-probe"><dt>语言 / 时区</dt><dd>${safe(c.region.language)} / ${safe(c.region.timezone)}</dd><dt>系统环境</dt><dd>${safe(environment ? (environmentNames[environment.os] || environment.os) + ' ' + environment.osVersion + ' · 网页环境模拟' : '原生环境')}</dd><dt>窗口</dt><dd>${safe(c.viewport.width)} × ${safe(c.viewport.height)}</dd><dt>GPU 名称</dt><dd>${safe(graphics ? graphics.vendor + ' / ' + graphics.renderer : '原生信息')} · 待独立浏览器读回</dd><dt>WebGPU</dt><dd>${c.webgpu === 'disabled' ? '请求阻断原生适配器，待启动读回' : '原生适配器'}</dd><dt>联网</dt><dd>仅已确认的 Ultra 隧道；不可直连回退</dd><dt>数据</dt><dd>独立会话，保留本组件的数据</dd></dl><p class="muted">预览未应用设置。打开时会重新核对连接；不满足条件会显示原因。</p></div>` : ''}</article>`;
  };
  const browserPage = () => browserHostCard() + `<div class="privacy-card"><h2>Chrome / Edge 配套扩展</h2><p>发行包中的 <span class="mono">browser-extension</span> 目录提供本地扩展。进入 <span class="mono">chrome://extensions</span> 或 <span class="mono">edge://extensions</span>，开启开发者模式后选择“加载已解压的扩展程序”。</p><p>扩展提供 WebRTC 非代理 UDP 限制、预测加载和云建议控制、第三方 Cookie 限制、网页敏感权限、手动链接清理，以及本地浏览环境自测。每项单独启用、读回验证和撤销。</p><div class="privacy-note warning">扩展只作用于安装它的浏览器配置。WebRTC 限制不等于完全禁用；原生应用、其他浏览器、隐身配置及所有脚本上下文不能据此判定受到保护。未连接桌面桥，桌面应用不会猜测扩展状态。</div></div>
    <div class="privacy-card"><h2>当前内嵌 WebView 的信息可见性</h2><p>此检查只读取当前客户端的 WebView。本地显示，不联网、不请求 GPS、摄像头或麦克风，不读取真实 IP 或设备序列号；不能代替 Chrome / Edge 扩展页的检查。</p><button class="btn sm" id="privacy-probe" ${model.busy ? 'disabled' : ''}>运行本地检查</button>${model.probe ? '<dl class="privacy-probe">' + Object.keys(model.probe).map(key => `<dt>${safe(key)}${privacyHelp('只描述当前客户端内嵌 WebView 的这项返回值；接口存在不代表已发生跟踪，接口不可用不代表其他应用或浏览器受到保护。')}</dt><dd>${safe(model.probe[key])}</dd>`).join('') + '</dl>' : ''}</div>${controls().filter(c => c.category === 'browser').map(controlCard).join('')}`;
  const metadataList = blocks => Array.isArray(blocks) && blocks.length ? '<table class="privacy-data-table"><thead><tr><th>元数据块类型</th><th>字节数</th></tr></thead><tbody>' + blocks.map(block => `<tr><td>${safe(block.type)}${privacyHelp('仅显示元数据块类型和大小，不解析或展示 GPS、作者、文本等实际内容；这不是全部可关联信息的清单。')}</td><td>${safe(block.bytes)}</td></tr>`).join('') + '</tbody></table>' : '<p class="muted">未发现此工具识别的指定块；这不代表文件没有其他元数据或身份信息。</p>';
  const metadataToolCard = () => {
    const status = model.metadataStatus, selection = model.metadataPreview, preview = selection && selection.preview;
    const fields = selection && selection.fields || status && status.fields || [];
    const componentReady = !!(status && status.backend && status.backend.available === true);
    const disabled = model.metadataBusy ? 'disabled' : '';
    return `<article class="privacy-card privacy-metadata-tool"><div class="privacy-card-head"><h2>按字段检查与清理${privacyHelp('使用随包核验的 ExifTool，仅处理主动选择的 JPEG / PNG。默认只看字段存在情况；你再选择后才显示实际值。清理写新副本，原文件不改。')}</h2><span class="privacy-badge">${status && status.backend && status.backend.available ? '组件文件已核验' : '组件尚未确认'}</span></div>
      <p>选择字段后查看或清理。支持 EXIF 作者、版权、描述、软件、相机、时间和 GPS。</p>
      <div class="privacy-actions"><button class="btn sm" id="privacy-metadata-status" ${disabled}>检查组件</button><button class="btn sm" id="privacy-metadata-select" ${disabled || !status || !status.backend || !status.backend.available ? 'disabled' : ''}>选择图片并读取字段</button></div>
      ${status && status.backend && !status.backend.available ? '<p class="privacy-note">需要安装包含 ExifTool 的完整 Ultra 包；下方基础图片工具仍可单独使用。</p>' : ''}
      ${preview ? `<p class="muted">${safe(preview.format)} · ${safe(preview.width)} × ${safe(preview.height)} · ${safe(preview.bytes)} 字节</p><div class="privacy-metadata-fields">${fields.map(field => { const count = Object.values(preview.tags || {}).filter(tag=>tag.field===field.id).length; return `<label><input type="checkbox" data-metadata-field="${safe(field.id)}" ${model.metadataSelected.includes(field.id) ? 'checked' : ''} ${disabled}> ${safe(field.label)} · ${count ? safe(count) + ' 个字段' : '未发现'}${privacyHelp('选择只改变待操作清单。查看才在本页显示实际值；清理只移除这一类支持的 EXIF 标签，不包括同名 XMP/IPTC 字段。')}</label>`; }).join('')}</div><div class="privacy-actions"><button class="btn sm" id="privacy-metadata-values" ${disabled || !componentReady || !model.metadataSelected.length ? 'disabled' : ''}>查看所选字段值</button><button class="btn sm primary" id="privacy-metadata-copy" ${disabled || !componentReady || !model.metadataSelected.length ? 'disabled' : ''}>清理所选并另存副本</button></div>` : ''}
      ${model.metadataError ? `<p class="privacy-error" role="alert">${safe(model.metadataError)}</p>` : ''}
      ${model.metadataValues ? `<dl class="privacy-probe privacy-metadata-values">${Object.entries(model.metadataValues).map(([key,value])=>`<dt>${safe(key)}</dt><dd>${safe(typeof value === 'string' ? value : JSON.stringify(value))}</dd>`).join('') || '<dt>没有找到所选字段</dt>'}</dl><p class="muted">实际值仅显示于本页，不加入检测或诊断导出。</p>` : ''}
      ${model.metadataCopy ? `<div class="privacy-metadata-copy" role="status"><p>已生成新副本并完成后验。实际移除：${safe((model.metadataCopy.removed || []).join('、') || '所选字段原本不存在')}。</p><details><summary>副本读回</summary><p>源图 SHA-256：${safe(model.metadataCopy.sourceSha256)}</p><p>副本 SHA-256：${safe(model.metadataCopy.copySha256)}</p></details></div>` : ''}
      <p class="muted">此工具仅选择处理 EXIF；XMP、IPTC、缩略图、文件时间及画面中的信息可能保留。需要基础块清理时可使用下方工具。</p></article>`;
  };
  const dataPage = () => `${metadataToolCard()}<article class="privacy-card"><h2>图片元数据检查</h2><p>手动选择一张本地 JPEG / PNG，只查看可识别元数据块的类型和大小。不会扫描文件夹、上传图片或展示 GPS 等具体内容。</p><div class="privacy-actions"><button class="btn sm" id="privacy-image-inspect" data-help-text="仅支持 JPEG / PNG。只检查你选中的文件，不修改原图；只返回块类型和字节数，不返回 GPS 等元数据内容。" ${model.busy || model.loading ? 'disabled' : ''}>选择图片并检查</button></div></article>
    <article class="privacy-card"><h2>清理指定元数据并另存</h2><p>选择源图片，再选择一个新的输出位置。只清理支持的元数据块，保留像素编码数据和 ICC 色彩配置，不覆盖原图或已有文件。</p><details class="privacy-note warning"><summary>分享副本前检查画面与方向</summary><p>人脸、文字、位置线索、文件名、ICC 与保留元数据仍可能关联身份。清理不改变画面；删除 EXIF 方向信息可能影响显示，请先检查副本。</p></details><div class="privacy-actions"><button class="btn sm primary" id="privacy-image-clean" data-help-text="仅处理 JPEG / PNG 的新副本，原图不改。保留像素和 ICC 色彩配置；仅移除指定块，不保证去除全部元数据或画面信息。取消文件选择不会生成副本。" ${model.busy || model.loading ? 'disabled' : ''}>选择图片并清理另存</button></div></article>
    ${model.dataReport ? `<article class="privacy-card"><div class="privacy-card-head"><h2>${safe(model.dataAction)}结果</h2><span class="privacy-badge">${safe(String(model.dataReport.format || '').toUpperCase())}</span></div><p class="privacy-last-check">本次本地处理：${safe(model.dataChecked)}</p><h3 class="privacy-data-subtitle">源图片中识别的指定块</h3>${metadataList(model.dataReport.metadataBlocks)}${model.dataAction === '清理' ? `<h3 class="privacy-data-subtitle">副本中已移除的指定块</h3>${metadataList(model.dataReport.removedBlocks)}` : ''}${model.dataReport.output ? `<dl><dt>新副本位置（仅本页显示）</dt><dd class="privacy-path">${safe(model.dataReport.output)}</dd></dl>` : ''}${model.dataReport.warning ? `<p class="privacy-note warning">${safe(model.dataReport.warning)}</p>` : ''}</article>` : ''}
    <article class="privacy-card"><h2>日志与诊断</h2><p>日志保留使用现有设置，诊断查看使用现有日志页面。数据工具不额外保存选图路径或元数据内容。</p><div class="privacy-actions"><button class="btn sm" data-privacy-nav="settings" data-help-text="前往原有应用设置查看日志相关选项，不在隐私中心重复修改或另设一套保留策略。">日志保留设置</button><button class="btn sm" data-privacy-nav="logs" data-help-text="前往诊断页面，选择报告内容并预览后再保存；详细项目可能含敏感信息，分享前仍应审阅。">查看诊断日志</button><button class="btn sm" data-privacy-nav="detection" data-help-text="前往检测中心，使用独立的脱敏报告导出。不会自动把图片信息、文件路径或完整原始日志加入报告。">检测报告与脱敏导出</button></div></article>`;
  const capabilitiesPage = () => {
    const caps = model.report && Array.isArray(model.report.capabilities) ? model.report.capabilities : [];
    const capabilityHint = c => c.status === 'unsupported' ? '应用无法修改此项；以下方案用于降低暴露，不代表已经阻止读取。' : c.status === 'manual' ? '应用只提供说明和系统入口；完成后请回到检测中心重新检查。' : c.status === 'limited' ? '只覆盖列出的范围，其他来源仍可能读取。' : c.status === 'planned' ? '尚未提供开关或自动处理，不会显示虚假的启用状态。' : '此处为能力范围说明，不等于系统已完成保护。';
    return `<div class="privacy-note">权限、配置值与实际隔离效果不同。无法确认的项目会保留为未知；应用不能统一关闭所有应用或网页的读取能力。登录账号、站点存储和内容行为仍可能关联身份。</div>${caps.map(c => `<article class="privacy-card"><div class="privacy-card-head"><h2>${safe(c.title || c.id)}${privacyHelp(c.guide || c.description || c.detail || c.scope || '此项目的生效范围取决于系统、浏览器和应用自身支持。')}</h2><span class="privacy-badge">${safe(c.status ? label(c) : '范围说明')}</span></div><div class="privacy-scope">${safe(c.scope)}</div><p>${safe(c.description || c.detail)}</p><details><summary>能力范围与处理说明</summary><p>${safe(capabilityHint(c))}</p>${c.guide ? `<p class="muted">${safe(c.guide)}</p>` : ''}</details>${c.settingsId ? `<button class="btn sm" data-privacy-settings="${safe(c.settingsId)}">打开系统设置</button>` : ''}</article>`).join('') || '<p class="privacy-empty">读取本地报告后显示能力边界。</p>'}`;
  };
  function draw() {
    if (!alive()) return;
    if(model.regionCleanup){model.regionCleanup();model.regionCleanup=null;}
    const observer = model.report ? (typeof model.report.observerElevated === 'boolean' ? model.report.observerElevated ? '当前报告由已提升进程观察；其读取能力不等于普通应用。' : '当前报告由普通权限进程观察。' : '观察进程权限尚未提供。') : '';
    const admin = model.report ? observer + (model.report.admin ? '管理员执行能力已可用；权限不代表保护已生效。' : '机器级修改需要单独授权，未授权项目不视为生效。') : '管理员执行能力尚未检查';
    const helperNames = {not_requested:'尚未请求', authorizing:'授权中', connected:'已连接', cancelled:'用户取消', cancelled_or_unavailable:'授权未完成', unavailable:'当前不可用', lost:'已失联', unknown:'状态未确认', closed:'已关闭'};
    const helperState = model.helper && model.helper.state || 'unknown';
    el.innerHTML = `<section class="privacy-hero"><div class="privacy-eyebrow">GODUSEVPN ULTRA · WINDOWS</div><h1>隐私保护设置</h1><p class="muted small">按需调整保护，查看效果与剩余暴露。</p><div class="privacy-last-check">管理员助手：${safe(helperNames[helperState] || '状态未确认')}${model.checked ? ' · ' + safe(model.checked) : ''}</div><div class="privacy-toolbar"><button class="btn sm primary" id="privacy-refresh" ${model.loading || model.busy ? 'disabled' : ''}>${model.loading ? '正在检查…' : '重新检查'}</button><button class="btn sm" id="privacy-restore-all" ${model.loading || model.busy || !model.reportReadable || !controls().some(c => c.managed) ? 'disabled' : ''}>恢复系统策略</button><button class="btn sm" id="privacy-authorize" data-help-text="只启动独立管理员助手，不提升整个界面。重复点击不会重复授权；未知写入不重试，授权完成后先重新检查。" ${model.busy || model.loading || helperState === 'authorizing' || helperState === 'connected' ? 'disabled' : ''}>${helperState === 'authorizing' ? '等待授权…' : helperState === 'connected' ? '管理员已授权' : '请求管理员'}</button></div><details><summary>权限与生效范围</summary><p>${safe(admin)}</p><p>新增保护按需应用。可选隐私设置失败不会断开隧道、撤销原有防泄漏保护或切换直连；配置读回不等于所有应用都已受限。</p></details>${centerSummary()}${guardDiagnostic()}${model.error ? `<div role="alert" class="privacy-error">${safe(model.error)}</div>` : ''}${model.actionNote ? `<p class="privacy-note" role="status">${safe(model.actionNote)}</p>` : ''}</section>
    ${(model.report && model.report.warnings || []).length ? `<details class="privacy-warnings"><summary>报告限制与提示（${model.report.warnings.length}）</summary>${model.report.warnings.map(w => `<p class="muted small">${safe(w)}</p>`).join('')}</details>` : ''}
    <div class="privacy-tabs" role="tablist">${[['system', '系统设置'], ['network', '程序联网'], ['browser', '浏览器保护'], ['data', '数据工具'], ['boundaries', '能力与引导']].map(pair => `<span class="privacy-tab-option"><button role="tab" aria-selected="${model.tab === pair[0]}" class="${model.tab === pair[0] ? 'active' : ''}" data-privacy-tab="${pair[0]}">${pair[1]}</button></span>`).join('')}</div><section id="privacy-panel">${model.tab === 'system' ? systemPage() : model.tab === 'network' ? firewallPage() : model.tab === 'browser' ? browserPage() : model.tab === 'data' ? dataPage() : capabilitiesPage()}${model.tab !== 'browser' && tabGuides() ? `<details class="privacy-module-guide-group"><summary>功能说明与处理入口</summary><p class="privacy-section-intro">当前状态以实际检查与操作回执为准。</p>${tabGuides()}</details>` : ''}</section><p class="privacy-footer">检查数据仅保留在此页面内存中。应用并读回后仍需按卡片范围验证；退出应用后系统设置可能继续生效，恢复操作只处理本应用的记录。</p>`;
    el.querySelectorAll('[data-privacy-tab]').forEach(button => button.addEventListener('click', () => { context.cancelPending();if(model.tab==='system'&&button.dataset.privacyTab!=='system'){cancelPortalObservation();model.portalGeneration++;model.portalPollGeneration++;} if(model.tab==='browser'&&button.dataset.privacyTab!=='browser'){model.browserProfileGeneration++;model.engineDeleteGeneration++;}if (model.tab === 'data' && button.dataset.privacyTab !== 'data') { model.metadataGeneration++; model.metadataValues=null; model.metadataCopy=null; } model.tab = button.dataset.privacyTab; draw(); }));
    el.querySelectorAll('[data-privacy-guide-action]').forEach(button => button.addEventListener('click', async () => {
      if (!alive() || !el.contains(button) || model.busy || model.loading) return;
      const action={kind:button.dataset.privacyGuideAction,target:button.dataset.guideTarget,label:button.textContent};
      if (!privacyGuideActionAllowed(action)) return;
      if (action.kind === 'privacy-tab') { context.cancelPending();if(model.tab==='system'&&action.target!=='system'){cancelPortalObservation();model.portalGeneration++;model.portalPollGeneration++;} model.tab=action.target; draw(); }
      else if (action.kind === 'app-page') nav(action.target);
      else try { await api('PrivacyOpenSettings',action.target); } catch (error) { if(alive())toast(errText(error),'err'); }
    }));
    el.querySelector('#privacy-refresh').addEventListener('click', refresh);
    el.querySelector('#privacy-authorize').addEventListener('click', authorize);
    el.querySelector('#privacy-restore-all').addEventListener('click', restoreAll);
    el.querySelectorAll('[data-privacy-apply]').forEach(button => button.addEventListener('click', () => apply(button.dataset.privacyApply)));
    el.querySelectorAll('[data-privacy-restore]').forEach(button => button.addEventListener('click', () => restore(button.dataset.privacyRestore)));
    el.querySelectorAll('[data-privacy-settings]').forEach(button => button.addEventListener('click', async () => { try { await api('PrivacyOpenSettings', button.dataset.privacySettings); } catch (error) { toast(errText(error), 'err'); } }));
    const search = el.querySelector('#privacy-search');
    if (search) search.addEventListener('input', () => { const position = search.selectionStart; model.search = search.value; draw(); const next = el.querySelector('#privacy-search'); next.focus(); if (position !== null) next.setSelectionRange(position, position); });
    const category = el.querySelector('#privacy-category');
    if (category) category.addEventListener('change', () => { model.category = category.value; draw(); });
    const timeChoice = el.querySelector('#privacy-time-choice');
    if (timeChoice) timeChoice.addEventListener('change', () => { model.timeDraft = timeChoice.value; model.timeMessage = ''; draw(); const current = el.querySelector('#privacy-time-choice'); if (current) current.focus(); });
    const timeRefresh = el.querySelector('#privacy-time-refresh');
    if (timeRefresh) timeRefresh.addEventListener('click', refreshTime);
    const timeApply = el.querySelector('#privacy-time-apply');
    if (timeApply) timeApply.addEventListener('click', applyTime);
    const timeRestore = el.querySelector('#privacy-time-restore');
    if (timeRestore) timeRestore.addEventListener('click', restoreTime);
    const choose = el.querySelector('#privacy-program');
    if (choose) choose.addEventListener('click', async () => { try { const file = await api('PrivacyChooseProgram'); if (file) { model.program = file; draw(); } } catch (error) { toast(errText(error), 'err'); } });
    const add = el.querySelector('#privacy-firewall-add');
    if (add) add.addEventListener('click', addRule);
    const modeSelect = el.querySelector('#privacy-firewall-mode');
    if (modeSelect) { modeSelect.value = model.mode; modeSelect.addEventListener('change', () => { model.mode = modeSelect.value; context.cancelPending(); draw(); }); }
    el.querySelectorAll('[data-privacy-remove-rule]').forEach(button => button.addEventListener('click', () => removeRule(button.dataset.privacyRemoveRule)));
    const probe = el.querySelector('#privacy-probe');
    if (probe) probe.addEventListener('click', async () => { try { model.probe = await privacyLocalProbe(); draw(); } catch (error) { toast(errText(error), 'err'); } });
    bindBrowserProfile();
    bindSavedBrowserProfiles();
    bindBrowserProxy();
    bindMetadataTool();
    bindBrowserEngines();
    bindLocationPortal();
    const regionSlot=el.querySelector('#privacy-region-slot');if(regionSlot&&window.PrivacyRegionUI&&typeof window.PrivacyRegionUI.mount==='function')model.regionCleanup=window.PrivacyRegionUI.mount({Report:()=>api('PrivacyRegionReport'),Apply:code=>api('PrivacyRegionApply',code),Restore:()=>api('PrivacyRegionRestore'),alive:()=>alive()&&model.tab==='system'&&el.contains(regionSlot),busy:()=>model.busy||model.loading||privacyWork.busy,confirm:context.confirm,toast,help:privacyHelp,decorate:()=>privacyDecorateHelp(regionSlot),draft:model.regionDraft||(model.regionDraft={code:'',touched:false})},regionSlot);
    const inspectImage = el.querySelector('#privacy-image-inspect');
    if (inspectImage) inspectImage.addEventListener('click', () => runData('PrivacyInspectImage', '检查'));
    const cleanImage = el.querySelector('#privacy-image-clean');
    if (cleanImage) cleanImage.addEventListener('click', () => runData('PrivacyCleanImage', '清理'));
    el.querySelectorAll('[data-privacy-nav]').forEach(button => button.addEventListener('click', () => nav(button.dataset.privacyNav)));
    privacyDecorateHelp(el);
    if (model.tab === 'browser') layoutBrowserEditor();
    if (arg && arg.controlId && controls().some(c => c.id === arg.controlId && c.category === 'browser') && model.tab !== 'browser') { model.tab = 'browser'; draw(); return; }
    if (arg && arg.controlId) { const target = Array.from(el.querySelectorAll('[data-privacy-control]')).find(card => card.dataset.privacyControl === arg.controlId); if (target) { target.scrollIntoView({ block: 'center' }); arg = null; } }
  }
  function layoutBrowserEditor() {
    const panel=el.querySelector('#privacy-panel'), profile=panel && panel.querySelector('.privacy-browser-profile');
    if(!profile)return;
    const hero=el.querySelector('.privacy-hero');
    if(hero){const details=document.createElement('details'),summary=document.createElement('summary');details.className='privacy-browser-context';summary.textContent='系统权限与本地检测';summary.dataset.helpText='查看本地报告、管理员助手和系统恢复入口；浏览器表单的编辑不需要在这里提交系统修改。';details.appendChild(summary);hero.parentNode.insertBefore(details,hero);details.appendChild(hero);const warnings=el.querySelector('.privacy-warnings');if(warnings)details.appendChild(warnings);details.open=!!model.browserContextOpen;details.addEventListener('toggle',()=>{model.browserContextOpen=details.open;});}
    const cards=Array.from(panel.children), fields=Array.from(profile.querySelectorAll('.privacy-browser-fields'));
    if(fields.length!==2)return;
    const title=profile.querySelector('.privacy-card-head h2');if(title && title.firstChild)title.firstChild.nodeValue='浏览器环境';
    const intro=profile.querySelector(':scope > p');if(intro)intro.textContent='选择环境、填写参数，再打开浏览器。';
    const navigation=document.createElement('div');navigation.className='privacy-editor-tabs';navigation.setAttribute('role','tablist');navigation.setAttribute('aria-label','浏览器配置分类');
    const pairs=[['environment','环境与身份'],['fingerprint','指纹设置'],['proxy','代理连接'],['versions','内核版本'],['permissions','现有浏览器']];
    const panes={};
    for(const [id,name] of pairs){
      const button=document.createElement('button');button.type='button';button.className='privacy-editor-tab';button.id='privacy-editor-tab-'+id;button.dataset.browserPane=id;button.setAttribute('role','tab');button.setAttribute('aria-controls','privacy-editor-pane-'+id);button.textContent=name;button.dataset.helpText='切换'+name+'，保留当前填写内容；不会修改系统或启动浏览器。';navigation.appendChild(button);
      const section=document.createElement('section');section.id='privacy-editor-pane-'+id;section.className='privacy-editor-pane';section.dataset.browserPanePanel=id;section.setAttribute('role','tabpanel');section.setAttribute('aria-labelledby',button.id);panes[id]=section;
    }
    panes.environment.appendChild(fields[0]);
    const subtitle=profile.querySelector('.privacy-data-subtitle');if(subtitle)panes.fingerprint.appendChild(subtitle);panes.fingerprint.appendChild(fields[1]);
    const proxy=panel.querySelector('.privacy-browser-proxy'), engines=panel.querySelector('.privacy-engine-manager');
    if(proxy)panes.proxy.appendChild(proxy);if(engines)panes.versions.appendChild(engines);
    for(const card of cards)if(card!==profile && card!==proxy && card!==engines)panes.permissions.appendChild(card);
    const footer=document.createElement('div');footer.className='privacy-editor-footer';
    for(const child of Array.from(profile.children))if(!child.matches('.privacy-card-head') && child!==intro)footer.appendChild(child);
    profile.classList.add('privacy-browser-editor');profile.appendChild(navigation);for(const [id]of pairs)profile.appendChild(panes[id]);profile.appendChild(footer);
    const selectPane=id=>{model.browserPane=id;for(const button of navigation.querySelectorAll('[data-browser-pane]')){const active=button.dataset.browserPane===id;button.classList.toggle('active',active);button.setAttribute('aria-selected',String(active));button.tabIndex=active?0:-1;}for(const [name,pane]of Object.entries(panes))pane.hidden=name!==id;};
    for(const button of navigation.querySelectorAll('[data-browser-pane]')){
      button.addEventListener('click',()=>selectPane(button.dataset.browserPane));
      button.addEventListener('keydown',event=>{const index=pairs.findIndex(([id])=>id===model.browserPane);let next=index;if(event.key==='ArrowRight')next=(index+1)%pairs.length;else if(event.key==='ArrowLeft')next=(index+pairs.length-1)%pairs.length;else if(event.key==='Home')next=0;else if(event.key==='End')next=pairs.length-1;else return;event.preventDefault();selectPane(pairs[next][0]);navigation.querySelector('[data-browser-pane="'+pairs[next][0]+'"]').focus();});
    }
    for(const label of profile.querySelectorAll('.privacy-browser-fields > label')){
      const control=Array.from(label.children).find(node=>node.matches('input,select,textarea'));if(!control)continue;
      const caption=document.createElement('span');caption.className='privacy-setting-label';
      const value=document.createElement('div');value.className='privacy-setting-control';
      for(const node of Array.from(label.childNodes)){if(node===control || node.nodeType===1 && node.matches('datalist'))value.appendChild(node);else caption.appendChild(node);}
      label.appendChild(caption);label.appendChild(value);
    }
    const os=profile.querySelector('#privacy-browser-environment');
    if(os){
      const choices=document.createElement('div');choices.className='privacy-segmented privacy-os-choices';choices.setAttribute('role','group');choices.setAttribute('aria-label','操作系统环境');
      for(const option of Array.from(os.options)){
        const button=document.createElement('button');button.type='button';button.textContent=option.value==='native'?'本机':option.textContent.split(' · ')[0];button.dataset.browserOs=option.value;button.classList.toggle('selected',os.value===option.value);button.setAttribute('aria-pressed',String(os.value===option.value));button.disabled=os.disabled;button.dataset.helpText=option.textContent;button.addEventListener('click',()=>{os.value=option.value;os.dispatchEvent(new Event('change',{bubbles:true}));});choices.appendChild(button);
      }
      os.classList.add('privacy-native-select');os.parentNode.insertBefore(choices,os);
    }
    const width=profile.querySelector('#privacy-browser-width'),height=profile.querySelector('#privacy-browser-height');
    const osVersion=profile.querySelector('#privacy-browser-os-version'),environment=model.browser&&model.browser.config.environment;
    if(osVersion&&environment&&environment.os==='windows'){
      osVersion.parentNode.classList.add('privacy-inline-controls','privacy-windows-version');
      for(const [name,version]of [['Windows 10','10.0.0'],['Windows 11','13.0.0']]){const button=document.createElement('button');button.type='button';button.className='btn sm';button.textContent=name;button.dataset.browserWindowsVersion=version;button.disabled=osVersion.disabled;button.dataset.helpText='将平台版本填为 '+version+'；可以继续手动修改。UA 仍按现代规范保留 NT 10.0。';button.addEventListener('click',()=>{osVersion.value=version;osVersion.dispatchEvent(new Event('input',{bubbles:true}));});osVersion.parentNode.appendChild(button);}
    }
    if(width&&height){const widthLabel=width.closest('label'),heightLabel=height.closest('label'),group=width.parentNode;group.classList.add('privacy-resolution-values');const separator=document.createElement('span');separator.textContent='×';group.appendChild(separator);group.appendChild(height);if(widthLabel.firstElementChild.firstChild)widthLabel.firstElementChild.firstChild.nodeValue='分辨率';heightLabel.remove();}
    const kernel=panes.environment.querySelector('select[aria-label="专用浏览器内核"]');
    if(kernel){const manage=document.createElement('button');manage.type='button';manage.className='btn sm';manage.textContent='管理内核';manage.dataset.helpText='查看、下载和校验官方 Chromium / Firefox 多版本；切换标签保留环境草稿。';manage.addEventListener('click',()=>selectPane('versions'));kernel.parentNode.classList.add('privacy-inline-controls');kernel.parentNode.appendChild(manage);}
    const privacyOnly=footer.querySelectorAll(':scope > p.muted');
    if(privacyOnly.length){const details=document.createElement('details'),summary=document.createElement('summary');summary.textContent='作用范围与启动条件';details.appendChild(summary);for(const item of privacyOnly)details.appendChild(item);footer.appendChild(details);}
    selectPane(panes[model.browserPane]?model.browserPane:'environment');privacyDecorateHelp(el);scheduleBrowserIdentity();
  }
  function scheduleBrowserIdentity() {
    const bridge=App(), c=model.browser && model.browser.config;
    if(!c || !bridge || c.identity && c.identity.mode==='manual')return;
    const native=model.browserEngine.engineId!=='webview2',method=native?'BrowserEngineIdentity':'BrowserProfileIdentity';
    if(!Object.prototype.hasOwnProperty.call(bridge,method))return;
    if(native&&!model.browserEngine.version){const field=el.querySelector('#privacy-browser-ua');if(field){field.value='';field.placeholder='选择已下载内核版本后生成 UA';}return;}
    const key=JSON.stringify({identity:c.identity,environment:c.environment,engine:model.browserEngine});
    if(model.browserIdentityRequestKey===key)return;model.browserIdentityRequestKey=key;
    const generation=++model.browserIdentityGeneration;
    clearTimeout(model.browserIdentityTimer);
    const current=()=>alive() && model.browser && model.browserIdentityGeneration===generation && model.browser.config.identity.mode!=='manual';
    model.browserIdentityTimer=setTimeout(async()=>{
      if(!current())return;
      const status=el.querySelector('#privacy-browser-ua-status');if(status)status.textContent='正在生成 UA…';
      try {
        const preview=await api(method,JSON.stringify(native?browserEngineRequest(model.browserEngine.engineId,model.browserEngine.version,true):model.browser.config));
        if(!current() || !preview || typeof preview.mode!=='string')return;
        model.browserIdentityPreview=preview;
        const field=el.querySelector('#privacy-browser-ua');if(field){field.value=preview.userAgent || '';field.placeholder=preview.userAgentTemplate || '启动时读取真实浏览器 UA';}
        if(status && status.isConnected)status.textContent=preview.exactChromiumVersionKnown?'来自当前运行时身份；启动时重新核对':preview.userAgent?'已根据环境生成；启动时核对真实内核':'启动时读取真实浏览器身份';
      }catch(error){if(current() && status && status.isConnected)status.textContent='UA 暂未生成，请检查系统版本和配置；也可以手动填写。';}
    },200);
  }
  async function refresh() {
    if (model.loading || model.busy) return;
    context.cancelPending();
    const generation = model.readGeneration = (model.readGeneration || 0) + 1;
    model.loading = true; model.error = ''; draw();
    model.reportReadable = false; model.firewallReadable = false;
    try { model.report = await api('PrivacyReport'); model.reportReadable = true; model.checked = new Date().toLocaleString(); }
    catch (error) { model.error = '本地报告读取失败：' + errText(error); }
    // Older test bridges and non-Windows shells do not bind the optional
    // inventory method. Do not call an unknown bridge method: a permissive
    // proxy could otherwise turn this read-only view into a recorded action.
    const browserBridge = App();
    if (browserBridge && Object.prototype.hasOwnProperty.call(browserBridge, 'PrivacyLocationProviderRuntime')) {
      try { model.portalRuntime = await api('PrivacyLocationProviderRuntime'); } catch (_) { model.portalRuntime = null; }
    } else {
      model.portalRuntime = null;
    }
    if (browserBridge && Object.prototype.hasOwnProperty.call(browserBridge, 'PrivacyModuleGuides')) {
      try { model.guides = await api('PrivacyModuleGuides'); } catch (_) { model.guides = null; }
    }
    if (browserBridge && Object.prototype.hasOwnProperty.call(browserBridge, 'BrowserHostReport')) {
      try { model.host = await api('BrowserHostReport'); }
      catch (error) { model.host = { supported: false, engines: [], warnings: ['当前构建未提供运行时清单；不会把内嵌 WebView 当作独立浏览器。'] }; }
    } else {
      model.host = null;
    }
    if (browserBridge && Object.prototype.hasOwnProperty.call(browserBridge, 'BrowserProfileDefaults')) {
      try {
        const info = await api('BrowserProfileDefaults', model.browser && model.browser.config ? model.browser.config.profileId : 'private');
        if (!model.browser) {model.browser = info;model.browserIdentityPreview=info && info.identityPreview || null;}
        else { model.browser.hostAvailable = info.hostAvailable; model.browser.canLaunch = info.canLaunch === true; model.browser.status = info.status; model.browser.detail = info.detail; model.browser.regions = info.regions; model.browser.graphicsPresets = info.graphicsPresets; model.browser.environmentPresets=info.environmentPresets; }
        model.browserError = '';
      } catch (error) { model.browserPreview = null; model.browserError = errText(error); if (model.browser) { model.browser.hostAvailable = false; model.browser.canLaunch = false; } }
    }
    try { model.firewall = await api('PrivacyFirewallReport'); model.firewallReadable = true; model.firewallError = ''; }
    catch (error) { model.firewallError = '程序规则读取失败：' + errText(error); }
    try { model.helper = await api('PrivacyHelperStatus'); } catch (error) { model.helper = { state: 'unknown', detail: '此构建未提供管理员会话状态。' }; }
    model.timeError = '';
    try { await readTime(); } catch (error) { model.timeError = '时区读取失败：' + errText(error); }
    model.loading = false; draw();
    if (model.helper && model.helper.state === 'authorizing' && alive()) setTimeout(() => { if (alive() && !model.busy) refresh(); }, 1500);
    setTimeout(() => { if (alive() && model.readGeneration === generation) { model.reportReadable = false; model.firewallReadable = false; model.timeReadable = false; draw(); } }, 120000);
  }
  function browserSavedProfileStatus() {
    return (model.browserSavedRecord ? (model.browserSavedDirty ? '有未保存的修改。' : '配置已保存；启动时重新检查。') : '当前为未保存草稿。') + ' 代理认证只保留在本页内存。';
  }
  function markBrowserProfileEdited(invalidatePreview) {
    model.browserSavedDirty = true; model.browserMessage = '';
    const status = el.querySelector('#privacy-browser-profile-status'); if (status) status.textContent = browserSavedProfileStatus();
    const message = el.querySelector('#privacy-browser-message'); if (message) { message.hidden = true; message.textContent = ''; }
    if (!invalidatePreview) return;
    model.browserPreview = null; model.browserError = '';
    const error = el.querySelector('#privacy-browser-error'); if (error) { error.hidden = true; error.textContent = ''; }
    const ready = browserEngineReady();
    const launch = el.querySelector('#privacy-browser-launch'); if (launch) { launch.disabled = !ready; launch.textContent = '检查并打开'; }
    const proxyLaunch = el.querySelector('#privacy-proxy-launch'); if (proxyLaunch) proxyLaunch.disabled = !ready;
    const preview = el.querySelector('#privacy-browser-preview-result'); if (preview) preview.hidden = true;
  }
  function browserV2EnsureState() {
    const defaults={storage:{cookieFormat:'json',merge:false,cookieCount:0,cookieSessionOnly:true},tabs:{urls:[],startupMode:'restore'},proxy:{mode:'no-proxy',type:'none',provider:'IP2Location',accountRef:''},fingerprint:{webrtc:'disable-non-proxied-udp',timezone:'real',geo:'real',language:'real',uiLanguage:'by-language',resolution:'ua',fonts:'default',noise:{canvas:false,webglImage:false,audio:false,media:false,clientRects:false,speechVoices:false},webglMetadata:'real',webglVendor:'',webglRenderer:'',webgpu:'real',seed:''},advanced:{cpu:'real',ram:'real',deviceName:'real',mac:'real',dnt:'default',portScan:'enabled',hardwareAcceleration:'default',tls:'default',teamApps:'global',dataSync:'global',browserSettings:'global',randomFingerprint:false,allowedPorts:[]},readback:{state:'unknown',scope:'browser-profile',runtimeVerified:false,warnings:[]}};
    const merge=(base,value)=>{if(!value||typeof value!=='object'||Array.isArray(value))return base;Object.keys(value).forEach(key=>{if(value[key]&&typeof value[key]==='object'&&!Array.isArray(value[key]))base[key]=merge(base[key]&&typeof base[key]==='object'?base[key]:{},value[key]);else base[key]=value[key];});return base;};
    const incoming=model.browserExtras&&typeof model.browserExtras==='object'?model.browserExtras:{}; ['storage','tabs','proxy','fingerprint','advanced','readback'].forEach(key=>{if(typeof incoming[key]==='string'){try{incoming[key]=JSON.parse(incoming[key]);}catch(_){delete incoming[key];}}}); model.browserExtras=merge(defaults,incoming); model.browserMeta=model.browserMeta&&typeof model.browserMeta==='object'?model.browserMeta:{browser:'chromium',environmentName:'',group:'',remark:'',tags:[]};
    if(!['sunbrowser','flowerbrowser','chromium','firefox','webview2'].includes(model.browserMeta.browser))model.browserMeta.browser='chromium';
    if(!Array.isArray(model.browserMeta.tags))model.browserMeta.tags=[];
    return model.browserExtras;
  }
  function browserV2Path(path) { browserV2EnsureState(); const parts=String(path||'').split('.'); let root=model.browserExtras; if(parts[0]==='meta'){parts.shift();root=model.browserMeta;} for(const part of parts.slice(0,-1)){if(!root[part]||typeof root[part]!=='object')root[part]={};root=root[part];} return {root,key:parts[parts.length-1]}; }
  function browserV2Set(path,value) { const ref=browserV2Path(path); if(ref.root)ref.root[ref.key]=value; if(model.browser&&model.browser.config){const c=model.browser.config,fp=model.browserExtras.fingerprint||{};if(path==='fingerprint.seed')c.randomizationSeed=String(value||'');if(path==='fingerprint.webgpu')c.webgpu=value==='disabled'?'disabled':'runtime-default';if(path==='fingerprint.webglMetadata'){if(value==='real')delete c.graphics;else c.graphics={mode:'webgl-metadata',presetId:'manual',vendor:fp.webglVendor||'',renderer:fp.webglRenderer||''};c.webgpu=value==='custom'?'disabled':c.webgpu||'runtime-default';}if(path==='fingerprint.webglVendor'||path==='fingerprint.webglRenderer'){c.graphics={mode:'webgl-metadata',presetId:'manual',vendor:fp.webglVendor||'',renderer:fp.webglRenderer||''};c.webgpu='disabled';}} model.browserExtras.readback={state:'unknown',scope:'browser-profile',runtimeVerified:false,warnings:['配置已修改，尚未启动读回。']}; }
  function browserV2Get(path,fallback) { const ref=browserV2Path(path); return ref.root&&ref.root[ref.key]!==undefined?ref.root[ref.key]:fallback; }
  function browserV2ParseCookies(raw,format) {
    const text=String(raw||'').trim(); if(!text)return 0; if(text.length>512*1024)throw Error('Cookie 文本超过 512 KB。');
    if(format==='json'){let value;try{value=JSON.parse(text);}catch(_){throw Error('Cookie JSON 格式无效。');} if(Array.isArray(value))return value.length; if(value&&Array.isArray(value.cookies))return value.cookies.length; if(value&&typeof value==='object')return Object.keys(value).length; throw Error('Cookie JSON 应为数组或对象。');}
    const lines=text.split(/\r?\n/).map(line=>line.trim()).filter(line=>line&&!line.startsWith('#')); if(format==='netscape'){return lines.filter(line=>line.split('\t').length>=7).length;}
    return lines.filter(line=>line.includes('=')&&line.split('=')[0].trim()).length;
  }
  function browserV2Validate() {
    browserV2EnsureState(); const x=model.browserExtras, urls=Array.isArray(x.tabs.urls)?x.tabs.urls:[]; if(urls.length>20)throw Error('启动网址最多 20 个。');
    urls.forEach(url=>{if(typeof url!=='string'||url.length>2048||!/^https?:\/\/[^\s]+$/i.test(url))throw Error('启动网址必须是完整的 HTTP 或 HTTPS URL。');});
    if(String(model.browserMeta.environmentName||'').length>100||String(model.browserMeta.group||'').length>100||String(model.browserMeta.remark||'').length>2000)throw Error('环境名称、分组或备注超过长度限制。');
    const ports=Array.isArray(x.advanced.allowedPorts)?x.advanced.allowedPorts:[]; if(ports.length>32||ports.some(port=>!Number.isInteger(Number(port))||Number(port)<1||Number(port)>65535))throw Error('允许端口必须是 1–65535 的整数，最多 32 个。');
    if(Number(x.storage.cookieCount)>10000)throw Error('Cookie 数量超过 10000。'); return true;
  }
  function browserV2SaveExtras() { browserV2EnsureState(); const copy=JSON.parse(JSON.stringify(model.browserExtras)); if(copy.storage){delete copy.storage.values;delete copy.storage.raw;copy.storage.cookieSessionOnly=true;} if(copy.readback){copy.readback.state='unknown';copy.readback.runtimeVerified=false;} return copy; }
  function browserV2GenerateSeed() { const bytes=new Uint8Array(16); if(window.crypto&&window.crypto.getRandomValues)window.crypto.getRandomValues(bytes); else for(let i=0;i<bytes.length;i++)bytes[i]=Math.floor(Math.random()*256); return Array.from(bytes).map(value=>value.toString(16).padStart(2,'0')).join(''); }
  function browserV2LaunchGate() {
    browserV2Validate(); const x=model.browserExtras, unsupported=[];
    if((x.tabs.urls||[]).length)unsupported.push('启动标签页 URL');
    if(Number(x.storage.cookieCount)>0)unsupported.push('Cookie 导入');
    if(x.proxy&&x.proxy.mode&&x.proxy.mode!=='no-proxy')unsupported.push('代理来源策略');
    const fp=x.fingerprint||{}, n=fp.noise||{};
    if(fp.webrtc&&fp.webrtc!=='disable-non-proxied-udp')unsupported.push('WebRTC 多态策略');
    if(fp.geo&&fp.geo!=='real')unsupported.push('地理位置策略');
    if(fp.language&&fp.language!=='real')unsupported.push('语言策略');
    if(fp.fonts&&fp.fonts!=='default')unsupported.push('字体策略');
    if(Object.values(n).some(Boolean))unsupported.push('硬件噪音');
    const a=x.advanced||{};
    ['ram','deviceName','mac'].forEach(key=>{if(a[key]&&a[key]!=='real')unsupported.push(key.toUpperCase());});
    if(a.dnt&&a.dnt!=='default')unsupported.push('Do Not Track');
    if(a.portScan&&a.portScan!=='enabled')unsupported.push('端口扫描保护');
    if(a.hardwareAcceleration&&a.hardwareAcceleration!=='default')unsupported.push('硬件加速');
    if(a.tls&&a.tls!=='default')unsupported.push('TLS 特性');
    if(unsupported.length)throw Error('当前宿主尚未提供这些字段的可靠启动读回：'+unsupported.join('、')+'。配置仍已保存，但为避免静默丢弃，启动已停止。');
  }
  function bindBrowserV2Fields() {
    browserV2EnsureState();
    el.querySelectorAll('[data-browser-v2]').forEach(input=>{
      const path=input.dataset.browserV2, update=()=>{
        if(model.browserBusy||model.engineBusy)return; let value;
        if(input.type==='checkbox')value=!!input.checked;
        else if(path==='meta.tags')value=input.value.split(',').map(v=>v.trim()).filter(Boolean).slice(0,32);
        else if(path==='tabs.urls')value=input.value.split(/\r?\n/).map(v=>v.trim()).filter(Boolean).slice(0,20);
        else if(path==='advanced.allowedPorts')value=input.value.split(',').map(v=>v.trim()).filter(Boolean).slice(0,32).map(v=>Number(v));
        else value=input.value;
        browserV2Set(path,value); model.browserV2Error=''; model.browserV2Message='配置草稿已修改，尚未运行时读回。'; markBrowserProfileEdited(true);
      };
      input.addEventListener(input.tagName==='SELECT'||input.type==='checkbox'?'change':'input',update);
      if(input.tagName==='TEXTAREA'&&input.dataset.browserV2==='tabs.urls')input.addEventListener('blur',()=>{try{browserV2Validate();}catch(error){model.browserV2Error=errText(error);}});
    });
    const cookie=el.querySelector('[data-browser-v2-cookie]'); if(cookie)cookie.addEventListener('input',()=>{if(model.browserBusy)return;model.browserCookieDraft=cookie.value;try{const count=browserV2ParseCookies(cookie.value,browserV2Get('storage.cookieFormat','json'));browserV2Set('storage.cookieCount',count);model.browserV2Error='';model.browserV2Message='Cookie 已解析 '+count+' 个，仅保留在本次会话内存。';}catch(error){model.browserV2Error=errText(error);} });
    const generate=el.querySelector('#privacy-browser-v2-generate'); if(generate)generate.addEventListener('click',()=>{if(model.browserBusy)return;const seed=browserV2GenerateSeed();browserV2Set('fingerprint.seed',seed);browserV2Set('advanced.randomFingerprint',true);model.browserSeed=seed;model.browserV2Message='已生成新指纹种子；保存并启动后才能验证实际读回。';markBrowserProfileEdited(true);draw();});
    const validate=el.querySelector('#privacy-browser-v2-validate'); if(validate)validate.addEventListener('click',()=>{try{browserV2Validate();model.browserV2Error='';model.browserV2Message='配置校验通过；所有运行时字段仍需启动后读回。';}catch(error){model.browserV2Error=errText(error);model.browserV2Message='配置校验未通过，未启动浏览器。';}draw();});
  }
  function bindBrowserProfile() {
    for(const button of el.querySelectorAll('[data-browser-read-start]'))button.addEventListener('click',async()=>{
      if(!alive()||model.tab!=='browser'||!el.contains(button)||model.browserBusy||model.engineBusy)return;
      const id=button.dataset.browserReadStart;model.browserBusy=true;draw();
      try{let result=await api('BrowserEngineProfileStatus');if(!validBrowserStart(result,id))throw new Error('会话状态已变化。');model.browserStart=result;model.browserError='';if(['starting','cancelling'].includes(result.status.state))result=await watchBrowserStart(result,++model.browserStartGeneration);if(alive())model.browserMessage=browserStartMessage(result);}
      catch(error){model.browserError='会话状态仍无法确认，请关闭独立内核后重新检查。';}
      finally{model.browserBusy=false;if(alive())draw();}
    });
    for(const button of el.querySelectorAll('[data-browser-cancel-start]'))button.addEventListener('click',async()=>{
      const id=button.dataset.browserCancelStart;
      if(!alive()||model.tab!=='browser'||!el.contains(button)||model.browserStartCancelBusy||!model.browserStart||model.browserStart.startId!==id||!model.browserStart.canCancelStart)return;
      model.browserStartCancelBusy=true;draw();
      try{
        const result=await api('BrowserEngineProfileCancelStart',id);
        if(!validBrowserStart(result,id))throw new Error('浏览器取消状态无法确认。');
        if(model.browserStart&&model.browserStart.startId===id)model.browserStart=result;
      }catch(error){
        try{const current=await api('BrowserEngineProfileStatus');if(validBrowserStart(current,id)&&model.browserStart&&model.browserStart.startId===id)model.browserStart=current;}catch(_){}
        if(alive()&&model.browserStart&&model.browserStart.startId===id&&['starting','cancelling','status_unknown'].includes(model.browserStart.status.state))model.browserError='取消尚未确认，请等待当前状态或关闭会话后重新检查。';
      }
      finally{model.browserStartCancelBusy=false;if(alive())draw();}
    });
    const authorizeBrowser=el.querySelector('#privacy-browser-authorize');if(authorizeBrowser)authorizeBrowser.addEventListener('click',()=>{if(alive()&&model.tab==='browser'&&el.contains(authorizeBrowser)&&!model.browserBusy)authorize();});
    const edited = () => markBrowserProfileEdited(true);
    const engineChoice=el.querySelector('#privacy-browser-engine');
    if(engineChoice)engineChoice.addEventListener('change',async()=>{
      if(model.browserBusy)return;const engineId=engineChoice.value;
      model.browserEngine={engineId,version:''};if(engineId!=='webview2')model.browserRoute='proxy';
      model.browserIdentityGeneration++;model.browserIdentityRequestKey=null;model.browserIdentityPreview=null;edited();
      model.browserBusy=true;draw();
      try{if(engineId!=='webview2'){const records=await api('BrowserEngineVersions');if(alive()&&Array.isArray(records)){model.engineVersions=records;const first=records.find(v=>v.engineId===engineId&&v.treeVerified&&v.versionVerified);if(first)model.browserEngine.version=first.version;}}}
      catch(error){if(alive())model.browserError='本地内核清单读取失败，请到“内核版本”重新检查。';}
      finally{model.browserBusy=false;if(alive())draw();}
    });
    const engineVersion=el.querySelector('#privacy-browser-engine-version');if(engineVersion)engineVersion.addEventListener('change',()=>{if(model.browserBusy)return;model.browserEngine.version=engineVersion.value;model.browserIdentityGeneration++;model.browserIdentityRequestKey=null;model.browserIdentityPreview=null;edited();draw();});
    const identityMode=el.querySelector('#privacy-browser-identity-mode'),ua=el.querySelector('#privacy-browser-ua');
    if(identityMode)identityMode.addEventListener('change',()=>{if(model.browserBusy||!model.browser)return;const mode=identityMode.value;model.browser.config.identity=mode==='manual'?{mode,userAgent:ua ? ua.value : ''}:{mode};model.browserIdentityPreview=null;model.browserIdentityRequestKey=null;edited();draw();});
    if(ua)ua.addEventListener('input',()=>{if(model.browserBusy||!model.browser)return;model.browser.config.identity={mode:'manual',userAgent:ua.value};model.browserIdentityGeneration++;if(identityMode)identityMode.value='manual';const status=el.querySelector('#privacy-browser-ua-status');if(status)status.textContent='将使用手动填写的 UA';edited();});
    const uaGenerate=el.querySelector('#privacy-browser-ua-generate');if(uaGenerate)uaGenerate.addEventListener('click',()=>{if(model.browserBusy||!model.browser)return;model.browser.config.identity={mode:'generated'};model.browserIdentityPreview=null;model.browserIdentityRequestKey=null;edited();draw();});
    [['id', 'profileId'], ['width', 'width'], ['height', 'height']].forEach(pair => {
      const input = el.querySelector('#privacy-browser-' + pair[0]);
      if (input) input.addEventListener('input', () => { if (model.browserBusy || !model.browser) return; if (pair[0] === 'id') model.browser.config.profileId = input.value; else model.browser.config.viewport[pair[1]] = Number(input.value); edited(); });
    });
    const region = el.querySelector('#privacy-browser-region');
    if (region) region.addEventListener('change', () => { const selected = (model.browser.regions || []).find(r => r.presetId === region.value); if (selected && !model.browserBusy) { model.browser.config.region = JSON.parse(JSON.stringify(selected)); edited(); draw(); } });
    const seed = el.querySelector('#privacy-browser-seed');
    if (seed) seed.addEventListener('input', () => { model.browserSeed = seed.value; edited(); });
    const environmentChoice = el.querySelector('#privacy-browser-environment');
    if (environmentChoice) environmentChoice.addEventListener('change', () => {
      if (model.browserBusy || !model.browser) return;
      const c=model.browser.config, preset=(model.browser.environmentPresets || []).find(p=>p.config.devicePresetId===environmentChoice.value);
      if (environmentChoice.value==='native') { delete c.environment; c.viewport={width:Math.max(800,c.viewport.width),height:Math.max(600,c.viewport.height)}; }
      else if (preset) {
        c.environment=JSON.parse(JSON.stringify(preset.config)); c.viewport=JSON.parse(JSON.stringify(preset.recommendedViewport));
        if (c.environment.os!=='windows' && c.graphics && c.graphics.presetId!=='manual') { delete c.graphics; c.webgpu='runtime-default'; model.browserMessage='切换系统环境后已恢复原生 GPU，避免沿用 Windows 图形模板。'; }
      } else return;
      const message=model.browserMessage;edited();model.browserMessage=message;draw();
    });
    const osVersion=el.querySelector('#privacy-browser-os-version');
    if(osVersion)osVersion.addEventListener('input',()=>{if(model.browserBusy||!model.browser||!model.browser.config.environment)return;model.browser.config.environment.osVersion=osVersion.value;edited();scheduleBrowserIdentity();});
    const graphicsChoice = el.querySelector('#privacy-browser-graphics');
    if (graphicsChoice) graphicsChoice.addEventListener('change', () => {
      if (model.browserBusy || !model.browser) return;
      const c = model.browser.config, choice = graphicsChoice.value;
      if(model.browserEngine.engineId==='chrome-for-testing'&&choice!=='native')return;
      if (choice === 'native') { delete c.graphics; c.webgpu = 'runtime-default'; }
      else {
        const preset = (model.browser.graphicsPresets || []).find(g => g.presetId === choice);
        if (choice !== 'manual' && !preset) return;
        c.graphics = preset ? JSON.parse(JSON.stringify(preset)) : { mode: 'webgl-metadata', presetId: 'manual', vendor: c.graphics && c.graphics.vendor || '', renderer: c.graphics && c.graphics.renderer || '' };
        c.webgl = 'runtime-default'; c.webgpu = 'disabled';
      }
      edited(); draw();
    });
    [['vendor', 'vendor'], ['renderer', 'renderer']].forEach(pair => {
      const input = el.querySelector('#privacy-browser-gpu-' + pair[0]);
      if (input) input.addEventListener('input', () => {
        if (model.browserBusy || !model.browser || !model.browser.config.graphics) return;
        model.browser.config.graphics[pair[1]] = input.value; model.browser.config.graphics.presetId = 'manual';
        if (graphicsChoice) graphicsChoice.value = 'manual'; edited();
      });
    });
    const webgpu = el.querySelector('#privacy-browser-webgpu');
    if (webgpu) webgpu.addEventListener('change', () => { if (!model.browserBusy && model.browser && ['runtime-default', 'disabled'].includes(webgpu.value) && !(model.browserEngine.engineId==='chrome-for-testing'&&webgpu.value==='disabled') && (!model.browser.config.graphics || model.browser.config.graphics.mode !== 'webgl-metadata')) { model.browser.config.webgpu = webgpu.value; edited(); } });
    const cpuMode=el.querySelector('#privacy-browser-cpu-mode');if(cpuMode)cpuMode.addEventListener('change',()=>{if(model.browserBusy||!model.browser)return;if(cpuMode.value==='native')delete model.browser.config.hardware;else model.browser.config.hardware={mode:'logical-processors',logicalProcessors:8};edited();draw();});
    const cpu=el.querySelector('#privacy-browser-cpu');if(cpu)cpu.addEventListener('input',()=>{if(model.browserBusy||!model.browser||!model.browser.config.hardware)return;model.browser.config.hardware.logicalProcessors=Number(cpu.value);edited();});
    [['preview', 'preview'], ['random', 'random'], ['launch', 'launch'], ['close', 'close']].forEach(pair => { const button = el.querySelector('#privacy-browser-' + pair[0]); if (button) button.addEventListener('click', () => {if(alive()&&model.tab==='browser'&&el.contains(button))runBrowserProfile(pair[1]);}); });
    bindBrowserV2Fields();
  }
  function bindSavedBrowserProfiles(){
    const name=el.querySelector('#privacy-browser-profile-name');if(name)name.addEventListener('input',()=>{if(!alive()||model.tab!=='browser'||!el.contains(name)||model.browserBusy)return;model.browserProfileName=name.value;markBrowserProfileEdited(false);});
    const exitCleanup=el.querySelector('#privacy-browser-exit-cleanup');if(exitCleanup)exitCleanup.addEventListener('change',()=>{if(!alive()||model.tab!=='browser'||!model.browser||model.browserBusy)return;model.browser.config.exitCleanup=exitCleanup.value==='delete-owned-profile'?'delete-owned-profile':'retain-profile';markBrowserProfileEdited(true);draw();});
    const select=el.querySelector('#privacy-browser-profile-select');if(select)select.addEventListener('change',()=>{model.browserSavedSelection=select.value;const button=el.querySelector('#privacy-browser-profile-load');if(button)button.disabled=!select.value;});
    for(const action of ['save','list','load','new','delete']){
      const button=el.querySelector('#privacy-browser-profile-'+action);if(!button)continue;
      button.addEventListener('click',async()=>{
        if(!alive()||model.tab!=='browser'||!el.contains(button)||model.browserBusy||!model.browser)return;
        const expectedGeneration=model.browserProfileGeneration;
        browserV2EnsureState(); const existing=model.browserSavedRecord,config=JSON.parse(JSON.stringify(model.browser.config)),engine=JSON.parse(JSON.stringify(model.browserEngine)),name=model.browserProfileName,meta=JSON.parse(JSON.stringify(model.browserMeta)),extras=browserV2SaveExtras();
        if(action==='delete'&&(!existing||!await context.confirm('删除“'+existing.name+'”的配置记录？\n仅删除此条保存的参数和历史记录，浏览数据保留。',{ok:'删除记录',danger:true})))return;
        if(action==='load'&&model.browserSavedDirty&&!await context.confirm('载入所选配置会替换当前未保存草稿。',{ok:'载入配置',danger:false}))return;
        if(!alive()||model.tab!=='browser'||!el.contains(button)||expectedGeneration!==model.browserProfileGeneration||model.browserBusy)return;
        const generation=++model.browserProfileGeneration,current=()=>alive()&&model.tab==='browser'&&model.browserProfileGeneration===generation;
        model.browserBusy=true;model.browserError='';model.browserMessage='';draw();
        try{
          if(action==='list'){const result=await api('BrowserProfilesList');if(!result||!Array.isArray(result.records))throw Error('配置清单读取失败。');if(current())model.browserSavedList=result.records;}
          else if(action==='delete'){const result=await api('BrowserProfileDelete',existing.id,existing.revision);if(!result||result.id!==existing.id||result.deleted!==true)throw Error('配置删除结果无法确认。');if(current()){model.browserSavedRecord=null;model.browserSavedDirty=true;model.browserSavedSelection='';model.browserSavedList=(model.browserSavedList||[]).filter(r=>r.id!==existing.id);model.browserMessage='配置记录已删除，当前草稿和浏览数据保留。';}}
          else{
            let result;
            if(action==='load')result=await api('BrowserProfileLoad',model.browserSavedSelection);
            else{browserV2Validate();delete config.binding;const record={schemaVersion:2,name,meta,extras,config,engine};if(action==='save'&&existing){record.id=existing.id;record.expectedRevision=existing.revision;}result=await api('BrowserProfileSave',JSON.stringify(record));}
            if(!result||!/^p-[0-9a-f]{32}$/.test(result.id||'')||!/^([0-9a-f]{64})$/.test(result.revision||'')||!result.config||!result.engine||result.config.profileId!==result.id)throw Error('保存配置结果无法确认。');
            if(current()){model.browserSavedRecord=result;model.browserProfileName=result.name;model.browserMeta=result.meta||{browser:'chromium',environmentName:'',group:'',remark:'',tags:[]};model.browserExtras=result.extras||model.browserExtras;browserV2EnsureState();model.browserCookieDraft='';model.browser.config=result.config;model.browserEngine=result.engine;model.browserSavedDirty=false;model.browserSavedSelection=result.id;model.browserPreview=null;model.browserIdentityPreview=null;model.browserIdentityRequestKey=null;model.browserIdentityGeneration++;if(result.engine.engineId!=='webview2')model.browserRoute='proxy';model.browserMessage=action==='load'?'配置已载入，启动时重新检查。':'配置已保存。';if(model.browserSavedList)model.browserSavedList=model.browserSavedList.filter(r=>r.id!==result.id).concat([result]);}
          }
        }catch(error){if(current()){const detail=errText(error);model.browserError=detail.includes('browser_profile_revision_conflict')?'保存记录已发生变化。请重新载入或另存为新配置；当前草稿保留。':detail.includes('browser_profile_input_refused')?'请检查配置名称、内核版本和参数格式，再重新保存。':detail.includes('browser_profile_store_busy')?'配置记录正在被使用，请稍后重试。':detail.includes('browser_profile_store_untrusted_or_corrupt')?'配置记录未通过完整性检查，请重新读取并选择其他记录，或另存当前草稿。':detail.includes('browser_profile_not_found')?'此配置记录已不存在，请重新读取清单。':detail;}}
        finally{model.browserBusy=false;if(alive())draw();}
      });
    }
  }
  function bindBrowserProxy() {
    const route=el.querySelector('#privacy-browser-route');if(route)route.addEventListener('change',()=>{if(!alive()||model.tab!=='browser'||!el.contains(route)||model.browserBusy||model.browserProxyBusy)return;model.browserRoute=route.value;});
    for (const field of ['type','host','port','username','password']) {
      const input = el.querySelector('#privacy-proxy-' + field);
      if (input) input.addEventListener(field === 'type' ? 'change' : 'input', () => {
        if (!alive() || model.tab!=='browser' || !el.contains(input) || model.browserProxyBusy || model.browserBusy) return;
        model.browserProxy[field] = field === 'port' ? Number(input.value) : input.value;
        model.browserProxyResult = null; model.browserProxyError = ''; model.browserProxyGeneration++;
        for (const oldResult of el.querySelectorAll('#privacy-proxy-result,.privacy-browser-proxy .privacy-probe')) oldResult.hidden = true;
        const oldError = el.querySelector('#privacy-proxy-error'); if (oldError) { oldError.hidden = true; oldError.textContent = ''; }
        const badge = el.querySelector('.privacy-browser-proxy .privacy-badge'); if (badge) badge.textContent = '尚未测试';
      });
    }
    const button = el.querySelector('#privacy-proxy-test');
    if (button) button.addEventListener('click', async () => {
      if (!alive() || model.tab!=='browser' || !el.contains(button) || model.browserProxyBusy) return;
      const generation = ++model.browserProxyGeneration;
      const config = JSON.stringify(model.browserProxy);
      model.browserProxyBusy = true; model.browserProxyResult = null; model.browserProxyError = ''; draw();
      try {
        const result = await api('BrowserProxyTest', config);
        if (!result || result.status !== 'reachable' || result.scope !== 'profile_proxy_probe_only' || result.provider !== 'country.is' || !result.ip || !result.country) throw new Error('代理未返回可核验的出口结果。');
        if (alive() && generation === model.browserProxyGeneration) model.browserProxyResult = result;
      } catch (error) { if (alive() && generation === model.browserProxyGeneration) model.browserProxyError = '代理测试未通过，请检查地址、认证与网络；没有改用直连。'; }
      finally { model.browserProxyBusy = false; if (alive()) draw(); }
    });
    const launch=el.querySelector('#privacy-proxy-launch');if(launch)launch.addEventListener('click',()=>{if(alive()&&model.tab==='browser'&&el.contains(launch))runBrowserProfile('launch-proxy');});
  }
  function browserEngineDeleteBlocked(){
    const state=model.browserStart&&model.browserStart.status&&model.browserStart.status.state;
    return model.engineBusy||model.browserBusy||model.browserProxyBusy||model.browserStartCancelBusy||!!(model.browserStart&&!['closed','start_cancelled','start_failed'].includes(state));
  }
  async function deleteBrowserEngineVersion(button,engineId,version){
    const present=()=>Array.isArray(model.engineVersions)&&model.engineVersions.some(v=>v.engineId===engineId&&v.version===version);
    if(!alive()||model.tab!=='browser'||!el.contains(button)||browserEngineDeleteBlocked()||!present())return;
    const generation=model.engineDeleteGeneration;
    if(!await context.confirm('删除 '+engineId+' '+version+' 的内核安装文件以释放空间？\n保留已保存配置与浏览数据。',{ok:'删除内核',danger:true}))return;
    if(!alive()||model.tab!=='browser'||!el.contains(button)||generation!==model.engineDeleteGeneration||browserEngineDeleteBlocked()||!present())return;
    const operation=++model.engineDeleteGeneration,current=()=>alive()&&model.tab==='browser'&&model.engineDeleteGeneration===operation;
    model.engineBusy=true;model.engineDeleting=true;model.engineError='';model.engineMessage='';draw();
    try{
      const state=await api('BrowserEngineProfileStatus');
      if(!current())return;
      if(!state||!state.status||!['closed','start_cancelled','start_failed'].includes(state.status.state)||state.status.ready!==false||state.status.canNavigate!==false||state.canCancelStart!==false)throw Error('engine_delete_session_not_closed');
      const result=await api('BrowserEngineDelete',engineId,version);
      if(!current())return;
      if(!result||result.engineId!==engineId||result.version!==version||result.deleted!==true)throw Error('engine_delete_result_unconfirmed');
      const versions=await api('BrowserEngineVersions');
      if(!current())return;
      if(!Array.isArray(versions)||versions.some(v=>!v||typeof v.engineId!=='string'||typeof v.version!=='string')||versions.some(v=>v.engineId===engineId&&v.version===version))throw Error('engine_delete_inventory_unconfirmed');
      model.engineVersions=versions;
      const selected=model.browserEngine.engineId===engineId&&model.browserEngine.version===version;
      if(selected){model.browserEngine.version='';model.browserIdentityGeneration++;model.browserIdentityPreview=null;model.browserIdentityRequestKey=null;markBrowserProfileEdited(true);}
      model.engineMessage='已删除 '+engineId+' '+version+' 的内核文件，配置与浏览数据保留。'+(selected?' 当前版本选择已清空，请重新下载并选择版本。':'');
    }catch(error){if(current())model.engineError='删除未确认，请先关闭浏览器，再检查本地版本。配置与浏览数据保留。';}
    finally{model.engineBusy=false;model.engineDeleting=false;if(alive())draw();}
  }
  function bindBrowserEngines(){
    for(const button of el.querySelectorAll('[data-browser-delete-engine]')){const engineId=button.dataset.browserDeleteEngine,version=button.dataset.browserDeleteVersion;button.addEventListener('click',()=>deleteBrowserEngineVersion(button,engineId,version));}
    for(const button of el.querySelectorAll('[data-browser-use-engine]'))button.addEventListener('click',()=>{if(model.engineBusy||model.browserBusy)return;model.browserEngine={engineId:button.dataset.browserUseEngine,version:button.dataset.browserUseVersion};model.browserRoute='proxy';model.browserPreview=null;model.browserIdentityPreview=null;model.browserIdentityRequestKey=null;model.browserIdentityGeneration++;model.browserPane='environment';draw();});
    for(const [id,key] of [['kind','engineKind'],['version','engineVersion'],['route','engineRoute']]){
      const input=el.querySelector('#privacy-engine-'+id);if(input)input.addEventListener(id==='version'?'input':'change',()=>{
        if(model.engineBusy)return;model[key]=input.value;model.engineError='';model.engineMessage='';
        if(id==='kind'){model.engineVersion='';draw();}else if(id==='version'){const download=el.querySelector('#privacy-engine-download'),launch=el.querySelector('#privacy-engine-launch');if(download)download.disabled=!model.engineVersion;if(launch)launch.disabled=!browserEngineManagerReady();}
      });
    }
    for(const action of ['refresh','download','local','launch','close']){
      const button=el.querySelector('#privacy-engine-'+action);if(button)button.addEventListener('click',async()=>{
        if(!alive()||model.tab!=='browser'||!el.contains(button)||model.engineBusy||model.browserBusy)return;
        const kind=model.engineKind,version=model.engineVersion,proxyJSON=model.engineRoute==='proxy'?JSON.stringify(model.browserProxy):'';
        if((action==='download'||action==='launch')&&!version)return;
        model.engineBusy=true;model.engineError='';model.engineMessage='';draw();
        try{
          if(action==='refresh'){const catalog=await api('BrowserEngineCatalog',proxyJSON);if(alive())model.engineCatalog=catalog;}
          else if(action==='local'){const versions=await api('BrowserEngineVersions');if(alive())model.engineVersions=versions;}
          else if(action==='download'){const result=await api('BrowserEngineDownload',JSON.stringify({engineId:kind,version,proxyJSON}));if(alive()){model.engineMessage='版本已下载，文件与版本读回已核对。启动隔离另行验证。';model.engineVersions=(model.engineVersions||[]).filter(v=>v.engineId!==result.engineId||v.version!==result.version).concat([result]);}}
          else if(action==='close'){await closeBrowserEngine();if(alive())model.engineMessage=model.browserStart?browserStartMessage(model.browserStart):'已请求关闭独立内核。';}
          else{
            const c=model.browser&&model.browser.config;
            const request=browserEngineRequest(kind,version);
            if(!proxyJSON)throw new Error('独立内核启动需填写本页代理并选择“本页独立代理”。');
            const result=await launchBrowserEngine(request,proxyJSON);if(alive())model.engineMessage=result.detail||'独立内核启动检查已返回，请核对窗口与状态。';
          }
        }catch(error){if(alive())model.engineError=errText(error);}
        finally{model.engineBusy=false;if(alive())draw();}
      });
    }
  }
  function browserEngineRequest(engineId,version,identityOnly=false){
    const c=model.browser&&model.browser.config;
    if(!c||!version)throw new Error('请先选择已下载的内核版本。');
    if(!identityOnly&&engineId!=='firefox'&&(c.graphics&&c.graphics.mode==='webgl-metadata'||c.webgpu!=='runtime-default'))throw new Error('此内核尚未支持所选 GPU / WebGPU 设置；本次不启动，也不会忽略这些参数。请先恢复原生图形信息或选择 Firefox / WebView2。');
    const request={engineId,version,profileId:c.profileId,language:c.region.language,timezone:c.region.timezone,width:c.viewport.width,height:c.viewport.height,identity:JSON.parse(JSON.stringify(c.identity)),exitCleanup:c.exitCleanup==='delete-owned-profile'?'delete-owned-profile':'retain-profile'};
    if(c.hardware){if(engineId==='firefox')throw new Error('Firefox 当前不支持自定义网页 CPU 核数；请恢复原生信息或选择 Chrome / WebView2。');request.hardware=JSON.parse(JSON.stringify(c.hardware));}
    if(!identityOnly&&engineId==='firefox'){if(c.graphics)request.graphics=JSON.parse(JSON.stringify(c.graphics));if(c.webgpu==='disabled')request.webgpu='disabled';}
    if(c.environment)request.environment=JSON.parse(JSON.stringify(c.environment));return request;
  }
  function browserStartControls(){
    if(!model.browserStart)return '';
    return `<span class="muted" role="status">${safe(browserStartMessage(model.browserStart))}</span>`+(model.browserStart.canCancelStart?`<button type="button" class="btn sm" data-browser-cancel-start="${safe(model.browserStart.startId)}" ${model.browserStartCancelBusy?'disabled':''}>${model.browserStartCancelBusy?'正在取消…':'取消启动'}</button>`:'')+(!['starting','cancelling','ready','closed','start_cancelled','start_failed'].includes(model.browserStart.status.state)?`<button type="button" class="btn sm" data-browser-read-start="${safe(model.browserStart.startId)}" ${model.browserBusy||model.engineBusy?'disabled':''}>检查会话状态</button>`:'');
  }
  function browserStartMessage(result){
    const state=result&&result.status&&result.status.state;
    const diagnostic=privacyGuardDiagnosticText(result&&result.status&&result.status.diagnostic || result&&result.diagnostic);
    const explain=text=>text+(diagnostic?' '+diagnostic:'');
    if(state==='start_failed')return explain({host_not_ready:'当前系统未提供受支持的独立浏览器宿主，请使用 Windows x64/ARM64 完整安装。',helper_required:'浏览器网络隔离尚不可用，请查看助手状态和阶段码。',version_store_unavailable:'内核缓存或下载不可用，请检查版本和代理连接。',graphics_readback_failed:'原生图形读回未通过，可能存在保护策略冲突或图形上下文不可用，本次启动已停止。',host_start_failed:'启动检查未通过，请检查浏览器组件和所选配置。'}[result.code]||'浏览器未启动，本次准备已结束。');
    if(state==='ready'&&result.status.graphics){const g=result.status.graphics;return g.implementation==='gecko-native-preferences'&&g.windowScopesVerified===4&&g.workerApiRuntimeReadback===false?'Firefox 原生图形已完成四类 Window 读回；Worker 执行已封锁，未作 Worker API 运行读回。':'浏览器已打开，图形证据范围请核对本次状态。';}
    return explain({starting:'正在准备浏览器，可以取消启动。',cancelling:'已请求取消，正在等待浏览器完成清理。',start_cancelled:'本次启动已取消。',start_failed:'浏览器未启动，本次准备已结束。',ready:'浏览器已打开并完成启动检查。',closed:'浏览器已关闭。',closed_filters_unknown:'浏览器已退出，网络规则清理仍未确认，请关闭会话后重新检查。',closing_or_unknown:'浏览器清理仍未确认，请关闭会话后重新检查。'}[state]||'浏览器状态尚未确认，请重新检查。');
  }
  function validBrowserStart(result,id){
    return !!(result&&/^[a-f0-9]{64}$/.test(result.startId||'')&&(!id||result.startId===id)&&typeof result.canCancelStart==='boolean'&&result.status&&typeof result.status.state==='string'&&!(result.status.state==='ready'&&(result.status.ready!==true||result.status.canNavigate!==true)));
  }
  async function launchBrowserEngine(request,proxyJSON){
    // Older embedded bridges retain their synchronous entry point. Current
    // builds use a scoped start token so cancellation never targets another run.
    const bridge=App();
    if(!Object.prototype.hasOwnProperty.call(bridge,'BrowserEngineProfileBeginLaunch'))return api('BrowserEngineProfileLaunch',JSON.stringify(request),proxyJSON);
    const generation=++model.browserStartGeneration;
    let result=await api('BrowserEngineProfileBeginLaunch',JSON.stringify(request),proxyJSON);
    if(!validBrowserStart(result))throw new Error('浏览器启动状态无法确认，请关闭独立内核后重新检查。');
    const id=result.startId;
    model.browserStart=result;
    if(alive())draw();
    return watchBrowserStart(result,generation);
  }
  async function watchBrowserStart(result,generation){
    const id=result.startId;
    while(['starting','cancelling'].includes(result.status.state)){
      if(!alive()||model.tab!=='browser'){
        if(result.canCancelStart)try{await api('BrowserEngineProfileCancelStart',id);}catch(_){}
        return result;
      }
      await new Promise(resolve=>setTimeout(resolve,600));
      if(generation!==model.browserStartGeneration)return result;
      if(!alive()||model.tab!=='browser'){
        if(result.canCancelStart)try{await api('BrowserEngineProfileCancelStart',id);}catch(_){}
        return result;
      }
      let next;
      try{next=await api('BrowserEngineProfileStatus');}
      catch(error){model.browserStart={...result,canCancelStart:false,status:{...result.status,state:'status_unknown',ready:false,canNavigate:false}};throw new Error('浏览器会话状态无法读取，请关闭独立内核后重新检查。');}
      if(!validBrowserStart(next,id)){model.browserStart={...result,canCancelStart:false,status:{...result.status,state:'status_unknown',ready:false,canNavigate:false}};throw new Error('浏览器会话状态已变化，请检查当前会话。');}
      const changed=next.status.state!==result.status.state||next.canCancelStart!==result.canCancelStart;
      result=next;model.browserStart=result;
      if(changed&&alive())draw();
    }
    result.detail=browserStartMessage(result);
    if(result.status.state==='start_failed')throw new Error(result.detail);
    return result;
  }
  async function closeBrowserEngine(){
    await api('BrowserEngineProfileClose');
    if(Object.prototype.hasOwnProperty.call(App(),'BrowserEngineProfileStatus')){
      const result=await api('BrowserEngineProfileStatus');
      if(validBrowserStart(result))model.browserStart=result;
      else if(result&&result.status&&result.status.state==='closed'&&!result.startId)model.browserStart=null;
      else throw new Error('关闭结果仍无法确认，请重新检查会话状态。');
    }
  }
  function bindMetadataTool() {
    for (const input of el.querySelectorAll('[data-metadata-field]')) input.addEventListener('change', () => {
      if (!alive() || model.tab !== 'data' || !el.contains(input) || model.metadataBusy) return;
      model.metadataGeneration++;
      model.metadataSelected = Array.from(el.querySelectorAll('[data-metadata-field]:checked')).map(item=>item.dataset.metadataField);
      model.metadataValues = null; model.metadataCopy = null;
      for (const button of el.querySelectorAll('#privacy-metadata-values,#privacy-metadata-copy')) button.disabled = !model.metadataSelected.length || !(model.metadataStatus && model.metadataStatus.backend && model.metadataStatus.backend.available === true);
      for (const old of el.querySelectorAll('.privacy-metadata-values,.privacy-metadata-copy')) old.hidden = true;
    });
    for (const [id,action] of [['status','status'],['select','select'],['values','values'],['copy','copy']]) {
      const button = el.querySelector('#privacy-metadata-' + id);
      if (button) button.addEventListener('click', async () => {
        if (!alive() || model.tab !== 'data' || !el.contains(button) || model.metadataBusy) return;
        const selection = model.metadataPreview, fields = model.metadataSelected.slice();
        if (action !== 'status' && !(model.metadataStatus && model.metadataStatus.backend && model.metadataStatus.backend.available === true)) return;
        if ((action==='values'||action==='copy') && (!selection || !fields.length)) return;
        const generation=++model.metadataGeneration;
        const current=()=>alive() && model.tab==='data' && generation===model.metadataGeneration;
        const hash=value=>typeof value==='string' && /^[a-f0-9]{64}$/i.test(value);
        model.metadataBusy = true; model.metadataError = ''; model.metadataValues=null; model.metadataCopy=null;
        if (action==='status') model.metadataStatus=null;
        if (action==='select') { model.metadataPreview=null; model.metadataSelected=[]; } draw();
        try {
          if (action==='status') { const status = await api('MetadataToolStatus'); if (!status || !status.backend || typeof status.backend.available!=='boolean' || !Array.isArray(status.fields)) throw new Error('invalid_metadata_status'); if (current()) { model.metadataStatus=status; if (!status.backend.available) {model.metadataPreview=null;model.metadataSelected=[];} } }
          else if (action==='select') { const result = await api('MetadataPreviewImage'); if (result && (!/^[a-f0-9]{32}$/.test(result.previewId || '') || !result.preview || !hash(result.preview.sourceSha256) || !Array.isArray(result.fields))) throw new Error('invalid_metadata_preview'); if (current() && result) model.metadataPreview=result; }
          else if (action==='values') { const result=await api('MetadataReadValues',selection.previewId,fields); if (!result || typeof result!=='object' || Array.isArray(result)) throw new Error('invalid_metadata_values'); if (current()) model.metadataValues=result; }
          else { const result=await api('MetadataCreateCopy',selection.previewId,fields); if (result && (result.sourceSha256!==selection.preview.sourceSha256 || !hash(result.copySha256) || !result.verified || result.verified.sourceSha256!==result.copySha256 || !Array.isArray(result.selected) || result.selected.length!==fields.length || !fields.every(field=>result.selected.includes(field)) || !Array.isArray(result.removed) || !result.removed.every(field=>fields.includes(field)))) throw new Error('invalid_metadata_copy_receipt'); if (current() && result) model.metadataCopy=result; }
        } catch (error) {
          if (current()) { const detail=errText(error); model.metadataError=detail.includes('source_changed')||detail.includes('预览已失效') ? '图片或预览已变化，请重新选择。原文件未修改。' : detail.includes('destination') ? '输出位置不接受，请选择新的本地文件名；不会覆盖已有文件。' : '操作未完成，请检查组件安装和文件权限。原文件未修改。'; }
        } finally { model.metadataBusy=false; if (alive()) draw(); }
      });
    }
  }
  async function runBrowserProfile(action) {
    if (!alive() || model.browserBusy || model.engineBusy || !model.browser) return;
    if(['launch','launch-proxy'].includes(action)){try{browserV2LaunchGate();}catch(error){model.browserV2Error=errText(error);model.browserV2Message='未启动：请先处理能力边界或等待运行时适配器。';draw();return;}}
    if(model.browserEngine.engineId!=='webview2'&&['preview','launch','launch-proxy','close'].includes(action)){
      const selected=JSON.parse(JSON.stringify(model.browserEngine)),proxyJSON=JSON.stringify(model.browserProxy),route=action==='launch-proxy'?'proxy':model.browserRoute;
      model.browserBusy=true;model.browserError='';model.browserMessage='';draw();
      try{
        if(action==='close'){await closeBrowserEngine();if(alive())model.browserMessage=model.browserStart?browserStartMessage(model.browserStart):'已请求关闭独立内核。';}
        else{const request=browserEngineRequest(selected.engineId,selected.version);if(action==='preview'){const result=await api('BrowserEngineIdentity',JSON.stringify(request));if(alive()){model.browserIdentityPreview=result;model.browserMessage='系统环境与 UA 检查完成，启动时核对实际内核与请求。';}}
          else{if(route!=='proxy')throw new Error('此独立内核当前需要在“代理连接”填写并选择本页独立代理。');const result=await launchBrowserEngine(request,proxyJSON);if(alive())model.browserMessage=result.detail||'本次内核启动状态已返回。';}}
      }catch(error){if(alive())model.browserError=errText(error);}
      finally{model.browserBusy=false;if(alive())draw();}return;
    }
    const launching=action==='launch'||action==='launch-proxy';
    const reviewed=launching && model.browserPreview && model.browserPreview.hostAvailable && model.browserPreview.canLaunch === true;
    let draft = JSON.stringify(reviewed ? model.browserPreview.config : model.browser.config);
    const proxyDraft=JSON.stringify(model.browserProxy);
    if(action==='random' && !model.browserSeed){const bytes=new Uint32Array(4);crypto.getRandomValues(bytes);model.browserSeed=Array.from(bytes,n=>n.toString(16).padStart(8,'0')).join('');}
    model.browserBusy = true; model.browserError = ''; model.browserMessage = ''; draw();
    try {
      if(launching && !reviewed){
        const result=await api('BrowserProfilePreview',draft,'');
        if(!alive())return;
        if(!result || !result.config || !result.hostAvailable || result.canLaunch !== true)throw new Error(result && result.detail || '当前构建未开放独立浏览器启动。');
        model.browser.config=result.config;model.browserPreview=result;model.browserIdentityPreview=result.identityPreview || model.browserIdentityPreview;
        draft=JSON.stringify(result.config);
      }
      if (action === 'close') { await api('BrowserProfileClose'); model.browserMessage = '浏览器已关闭，VPN 连接保持原状。'; }
      else if (action === 'launch') { const result = model.browserRoute==='proxy'?await api('BrowserProfileLaunchProxy',draft,proxyDraft):await api('BrowserProfileLaunch', draft); model.browserMessage = result.detail || '浏览器已打开。'; }
      else if(action==='launch-proxy'){const result=await api('BrowserProfileLaunchProxy',draft,proxyDraft);model.browserMessage=result.detail||'浏览器已使用此代理打开。';}
      else {
        const result = await api('BrowserProfilePreview', draft, action === 'random' ? model.browserSeed : '');
        if (!alive()) return;
        model.browser.config = result.config; model.browserPreview = result; model.browserIdentityPreview=result.identityPreview || model.browserIdentityPreview; model.browser.hostAvailable = result.hostAvailable; model.browser.canLaunch = result.canLaunch === true;
      }
    } catch (error) { model.browserError = errText(error); if (action !== 'close') model.browserPreview = null; }
    finally { model.browserBusy = false; if (alive()) draw(); }
  }
  async function authorize() {
    if (!alive() || model.busy || model.loading || privacyWork.busy) return;
    if (!await context.confirm('授权独立管理员助手？\n只用于固定机器策略和程序规则，不提升界面，不自动重试先前操作。', { ok: '请求授权', danger: false })) return;
    let authorizationError = '';
    model.busy = true; model.error = ''; model.helper = {state:'authorizing'}; draw();
    try { model.helper = await api('PrivacyAuthorize'); } catch (error) { authorizationError = '授权未完成：' + errText(error); model.error = authorizationError; }
    model.busy = false;
    if (alive()) { await refresh(); if (authorizationError) { model.error = authorizationError + (model.error ? ' ' + model.error : ''); draw(); } }
  }
  async function readTime() {
    model.timeReadable = false;
    const report = await api('PrivacyTimeReport');
    if (!report || typeof report.supported !== 'boolean' || typeof report.currentId !== 'string' || !Array.isArray(report.choices) || typeof report.daylightSavingDisabled !== 'boolean') throw new Error('没有返回可验证的时区报告，请使用 Windows 设置。');
    model.time = report;
    model.timeReadable = true;
    if (model.timeDraft === null || !timeChoices().some(choice => choice.id === model.timeDraft)) model.timeDraft = report.currentId;
    return report;
  }
  async function refreshTime() {
    if (model.busy || model.loading) return;
    model.busy = true; model.timeError = ''; model.timeMessage = ''; draw();
    try { await readTime(); } catch (error) { model.timeError = '时区读取失败：' + errText(error); }
    model.busy = false; draw();
  }
  async function applyTime() {
    const time = model.time, target = model.timeDraft, choice = timeChoices().find(entry => entry.id === target);
    if (model.busy || model.loading || model.timeError || !time || !time.supported || time.canApply === false || !choice || target === time.currentId || ['available', 'configured'].indexOf(time.status) < 0) return;
    const beforeDST = time.daylightSavingDisabled;
    if (!await context.confirm('将系统时区改为“' + choice.label + '”？\n日历和本地时间显示可能变化。保留现有夏令时设置，不修改 UTC 时钟、语言或隧道。', { ok: '应用所选时区', danger: false })) return;
    if (!alive()) return;
    model.busy = true; model.timeError = ''; model.timeMessage = ''; draw();
    let result = null;
    try { result = await api('PrivacyTimeApply', target); } catch (error) { model.timeError = '时区修改未完成：' + errText(error); }
    try {
      const after = await readTime();
      if (!model.timeError && after.currentId !== target) model.timeError = '时区调用已返回，但当前 ID 与所选时区不一致；未视为生效，请重新读取或恢复。';
      if (!model.timeError && typeof beforeDST === 'boolean' && after.daylightSavingDisabled !== beforeDST) model.timeError = '时区已读回，但夏令时偏好与操作前不一致；请核对并按需恢复，未视为完整成功。';
      if (!model.timeError) model.timeMessage = (result && result.message || '所选时区已读回。') + ' 当前 ID：' + after.currentId + '。浏览器覆盖值仍需单独复核。';
    } catch (error) { model.timeError = (model.timeError ? model.timeError + ' ' : '') + '修改后无法重新读取时区：' + errText(error); }
    if (model.timeError) toast(model.timeError, 'err'); else toast('系统时区 ID 已读回，夏令时偏好保持原值。');
    model.busy = false; draw();
  }
  async function restoreTime() {
    if (model.busy || model.loading || !model.timeReadable || !model.time || model.time.canApply === false || !model.time.managed) return;
    if (!await context.confirm('恢复本应用保存的原时区和夏令时偏好？\n日历和本地时间显示可能再次变化。发现外部修改时不会强行覆盖。', { ok: '恢复原时区' })) return;
    if (!alive()) return;
    model.busy = true; model.timeError = ''; model.timeMessage = ''; draw();
    let result = null;
    try { result = await api('PrivacyTimeRestore'); } catch (error) { model.timeError = '时区恢复未完成：' + errText(error); }
    try {
      const after = await readTime();
      if (!model.timeError && after.managed) model.timeError = '恢复调用已返回，但仍保留本应用的管理记录；请检查详情，未视为完成。';
      if (!model.timeError) { model.timeDraft = after.currentId; model.timeMessage = (result && result.message || '恢复调用已完成。') + ' 已重新读取当前 ID：' + after.currentId; }
    } catch (error) { model.timeError = (model.timeError ? model.timeError + ' ' : '') + '恢复后无法重新读取：' + errText(error); }
    if (model.timeError) toast(model.timeError, 'err'); else toast('恢复后已重新读取系统时区。');
    model.busy = false; draw();
  }
  async function mutate(method, arg) {
    if (model.busy || model.loading || !alive() || privacyWork.busy) return;
    model.busy = true; model.error = ''; draw();
    try {
      const result = await api(method, arg);
      const warning = result && (result.warning || (result.warnings || []).join('；') || (result.verified === false ? '操作已返回，但保护效果尚未验证。请查看重新检查结果。' : ''));
      const status = method === 'PrivacyFirewallRemove' && result && result.verified ? '规则已移除。' : method === 'PrivacyFirewallApply' && result && result.applied ? '过滤器已安装，流量待验证。' : result && result.message || '操作已返回，正在重新检查。';
      toast(status);
      if (warning) model.actionNote = '范围说明：' + warning;
    } catch (error) { model.error = '操作未完成：' + errText(error); }
    model.busy = false;
    // Preserve the error while reading current state; no navigation or connection changes.
    const priorError = model.error;
    if (!alive()) return;
    await refresh();
    if (priorError) { model.error = priorError; draw(); }
  }
  async function apply(id) {
    const control = controls().find(c => c.id === id);
    if (!control || !canApply(control) || model.busy || model.loading) return;
    const tab = model.tab;
    const ok = await context.confirm('应用“' + (control.title || id) + '”？\n\n范围：' + (control.scope || '以此项说明为准') + '\n影响：' + (control.impact || '可能影响依赖此功能的应用。') + '\n\n' + restartText(control), { ok: '应用并读回', danger: false });
    if (ok && model.tab === tab && canApply(control)) await mutate('PrivacyApply', id);
  }
  async function restore(id) {
    const control = controls().find(c => c.id === id);
    if (!control || model.busy || model.loading) return;
    const tab = model.tab;
    if (model.reportReadable && await context.confirm('恢复“' + (control.title || id) + '”修改前的设置？\n此操作可能重新允许信息读取。外部已修改的值会保留并提示冲突。', { ok: '恢复原值' }) && model.tab === tab) await mutate('PrivacyRestore', id);
  }
  async function restoreAll() {
    if (model.busy || model.loading) return;
    if (model.reportReadable && await context.confirm('恢复本应用记录的系统隐私策略？\n时区与程序规则需单独恢复，冲突项目不会强制覆盖。', { ok: '恢复系统策略' })) await mutate('PrivacyRestoreAll');
  }
  async function addRule() {
    if (model.busy || model.loading || !model.program || !firewallReady()) return;
    if (duplicateRule()) { toast('已有相同程序和范围的规则，请查看规则列表。'); return; }
    const mode = el.querySelector('#privacy-firewall-mode').value;
    const program = model.program;
    if (await context.confirm('为以下程序添加规则？\n' + program + '\n\n规则可能影响联网、设备发现或局域网服务。先退出并重新启动目标程序验证。', { ok: '添加并检查' }) && model.tab === 'network' && firewallReady() && model.program === program && model.mode === mode) await mutate('PrivacyFirewallApply', { program: program, mode: mode });
  }
  async function removeRule(id) {
    if (model.busy || model.loading) return;
    if (firewallReady() && await context.confirm('移除此程序规则？\n程序将恢复受其余防火墙和隧道规则决定的访问权限。', { ok: '移除规则' }) && model.tab === 'network') await mutate('PrivacyFirewallRemove', id);
  }
  async function runData(method, action) {
    if (model.busy || model.loading) return;
    model.busy = true; model.error = ''; draw();
    try {
      const report = await api(method);
      if (report && report.format) { model.dataReport = report; model.dataAction = action; model.dataChecked = new Date().toLocaleString(); toast(action === '清理' ? '副本已另存，请检查显示方向和剩余信息。' : '已完成本地检查，仅显示块类型与大小。'); }
      else toast('已取消文件选择；没有处理新的图片。');
    } catch (error) { model.error = '图片' + action + '未完成：' + errText(error); }
    model.busy = false; draw();
  }
  draw();
  refresh();
}

async function privacyLocalProbe() {
  const report = {};
  report['检查范围'] = '仅当前应用内嵌 WebView；未探测网络、真实 IP 或 GPS。';
  try { report['时区'] = Intl.DateTimeFormat().resolvedOptions().timeZone || '未返回'; } catch (error) { report['时区'] = '不可读取'; }
  report['语言'] = (navigator.languages || [navigator.language]).join(', ');
  report['User Agent'] = navigator.userAgent;
  report['屏幕 / DPR'] = screen.width + ' × ' + screen.height + ' / ' + window.devicePixelRatio;
  report['可见逻辑处理器 / 内存估计'] = String(navigator.hardwareConcurrency || '未提供') + ' / ' + (navigator.deviceMemory ? navigator.deviceMemory + ' GB（粗略）' : '未提供');
  report['Canvas / 字体接口'] = 'Canvas ' + (document.createElement('canvas').getContext('2d') ? '可用' : '不可用') + '；字体集 ' + (document.fonts ? '可用（未枚举）' : '不可用');
  const canvas = document.createElement('canvas');
  let gl;
  try {
    gl = canvas.getContext('webgl');
    if (gl) {
      const info = gl.getExtension('WEBGL_debug_renderer_info');
      report['WebGL'] = info ? String(gl.getParameter(info.UNMASKED_RENDERER_WEBGL)) : '可用；未开放调试渲染器名称';
    } else report['WebGL'] = '当前上下文不可用';
  } catch (error) { report['WebGL'] = '读取失败或被限制'; }
  if (gl) { const lose = gl.getExtension('WEBGL_lose_context'); if (lose) lose.loseContext(); }
  report['WebGPU / WebRTC'] = 'WebGPU ' + (navigator.gpu ? '接口存在（未创建设备）' : '接口不可用') + '；WebRTC ' + (window.RTCPeerConnection ? '接口存在（未建立连接）' : '接口不可用');
  report['剩余边界'] = '接口存在不代表已经发生跟踪；接口不可用也不代表无法通过其他特征识别。当前结果不能证明其他程序或网页受到保护。';
  return report;
}

function privacyHelp(text, label) {
  return '<button type="button" class="privacy-help" aria-label="' + esc((label || String(text || '项目').slice(0, 30)) + '：帮助') + '" data-privacy-help="' + esc(text || '查看此项的生效范围与影响。') + '">?</button>';
}

function privacyDecorateHelp(el) {
  el.querySelectorAll('h1, h2').forEach(heading => {
    if (heading.querySelector('.privacy-help')) return;
    const card = heading.closest('article, .privacy-card, .privacy-hero');
    const description = card && card.querySelector('p');
    heading.insertAdjacentHTML('beforeend', privacyHelp(description ? description.textContent : '查看此功能的作用范围、当前状态和可用操作。', heading.textContent.trim()));
  });
  const hints = {
    'privacy-refresh': '只读取本机配置和已记录的规则，不发送到在线检测站点。检查失败不会断开或降级隧道。',
    'privacy-restore-all': '恢复本应用管理的系统策略，遇到外部更改会保留并报告冲突。时区需使用时区卡片的专用恢复按钮，程序联网规则需要单独移除。',
    'privacy-search': '仅筛选当前报告，不修改设置，也不会把搜索内容发到网络。',
    'privacy-category': '按系统设置类别筛选，不改变任何保护。',
    'privacy-program': '选择目标可执行文件路径。此时尚未修改联网权限，点击确认添加后才生效。',
    'privacy-firewall-mode': '待应用。局域网规则限制指定地址范围；发现规则仅覆盖 IPv4/IPv6 组播和 IPv4 有限广播；不含定向广播、单播发现、蓝牙和系统代发。直接连接限制不保护回环、代理和转发路径。详细边界由后端报告说明。',
    'privacy-firewall-add': '保存并应用刚才选择的程序和范围。选择框本身不修改防火墙；不会修改原有隧道配置。',
    'privacy-probe': '只检查当前内嵌 WebView。无法据此推断 Chrome、Edge 或其他应用的实际暴露情况。',
    'detection-export': '导出当前检测快照，文件只保存在本机，不会自动上传。',
    'privacy-location-prepare': '准备本机的 Windows 定位组件并建立仅本机认证连接；不会开放局域网或无认证回退。',
    'privacy-location-read': '只读取组件、覆盖和恢复状态，不会应用新的坐标或轨迹。',
    'privacy-location-unprepare': '恢复本应用记录的组件准备；先恢复位置覆盖，遇到外部更改会停止并提示。',
    'privacy-location-apply': '确认后应用当前坐标或轨迹，并读回门户配置；这不是其他应用的消费者证据。',
    'privacy-location-observe': '通过独立 Windows 定位读取核对新鲜位置；结果仍只代表当前测试应用。',
    'privacy-location-restore': '恢复本应用保存的位置覆盖；不会删除系统中其他来源的缓存或覆盖。',
    'privacy-location-stop': '停止当前轨迹任务并保留可恢复的本应用记录。',
    'privacy-location-connect': '仅核验 HTTPS 127.0.0.1、证书指纹和本次认证信息，不连接外部地址。',
    'privacy-location-label': '仅作为本页的本地标签保存，不进行在线地理编码或上传。',
    'privacy-location-latitude': '填写 −90 至 90 的十进制纬度；输入只保留在本页草稿，确认后才应用。',
    'privacy-location-longitude': '填写 −180 至 180 的十进制经度；输入只保留在本页草稿，确认后才应用。',
    'privacy-location-trajectory': '每行填写纬度,经度；最多 120 点，输入不会自动执行。',
    'privacy-location-interval': '轨迹点间隔为 2–60 秒，总时长不超过 10 分钟。',
    'privacy-location-port': '仅接受本机 HTTPS 位置门户端口，默认 58443。',
    'privacy-location-pin': '填写 64 位小写 SHA-256 证书指纹，用于防止连接错误服务。',
    'privacy-location-user': '本机门户用户名，仅用于本次认证，不会写入配置。',
    'privacy-location-password': '本机门户密码仅在本次内存中使用，不写入配置或日志。'
    , 'privacy-proxy-test': '仅测试当前填写代理的连通性与出口信息；结果不代表专用浏览器已经完成真实导航。'
    , 'privacy-proxy-type': '选择代理协议，保存前不会修改系统或其他应用。'
    , 'privacy-proxy-host': '填写域名或 IP，不要包含协议、路径或用户名。'
    , 'privacy-proxy-port': '填写 1–65535 的代理端口；不会自动猜测端口。'
    , 'privacy-proxy-username': '可选的代理用户名，仅在本页内存中使用。'
    , 'privacy-proxy-password': '可选的代理密码，仅在本页内存中使用，不保存到配置。'
    , 'privacy-browser-launch': '重新检查环境、内核、代理和控制连接后启动专用浏览器；不会把预览当成真实运行证据。'
    , 'privacy-browser-close': '关闭当前专用浏览器会话并重新读取状态。'
    , 'privacy-browser-ua': '按环境生成的 UA 预览，可手动修改；启动时仍会核对真实内核。'
    , 'privacy-browser-ua-generate': '根据当前操作系统、内核和版本重新生成 UA 预览。'
    , 'privacy-browser-profile-save': '保存当前环境配置，不包含代理地址、密码、订阅或启动授权。'
    , 'privacy-browser-profile-list': '读取本机保存的配置清单，不启动浏览器。'
    , 'privacy-browser-profile-new': '保留当前记录并另存为新的配置。'
    , 'privacy-browser-profile-delete': '删除选中的配置记录；浏览器数据和已下载内核不受影响。'
    , 'privacy-browser-profile-load': '载入所选配置到当前草稿，未保存修改会先请求确认。'
    , 'privacy-engine-refresh': '重新读取已下载内核清单与校验状态。'
    , 'privacy-engine-download': '下载并校验选择的官方内核版本，完成后可在列表中删除以释放空间。'
    , 'privacy-engine-launch': '使用当前已核验的内核启动专用浏览器，并重新检查代理和运行时。'
    , 'privacy-engine-kind': '选择要读取和下载的官方内核系列。'
    , 'privacy-engine-version': '填写或选择官方目录中的准确版本；只接受可核验的版本。'
    , 'privacy-engine-route': '选择内核下载连接；独立代理只使用当前页面的内存参数。'
    , 'privacy-engine-local': '只读取本机已下载内核的文件清单和版本证据，不启动进程。'
    , 'privacy-engine-close': '关闭当前独立内核会话并重新读取生命周期状态。'
    , 'privacy-browser-engine': '选择专用浏览器实际使用的宿主内核。'
    , 'privacy-browser-engine-version': '选择已经下载并核验的具体内核版本。'
    , 'privacy-browser-profile-name': '给当前浏览器环境起一个本地配置名称。'
    , 'privacy-browser-exit-cleanup': '选择关闭专用会话后的数据处理方式；删除只作用于本组件拥有的数据树。'
    , 'privacy-browser-region': '选择语言、地区和时区组合，只影响专用浏览器环境。'
    , 'privacy-browser-environment': '选择网页可见的系统设备模板，可继续手动编辑。'
    , 'privacy-browser-os-version': '填写系统环境版本提示；不会改变真实 Windows 版本。'
    , 'privacy-browser-identity-mode': '选择 UA 生成、手动填写或保留原生 UA。'
    , 'privacy-browser-width': '设置浏览内容区域宽度，不更换物理显示器。'
    , 'privacy-browser-height': '设置浏览内容区域高度，过小或过大会被拒绝。'
    , 'privacy-browser-seed': '填写本地随机种子以生成成对的环境参数。'
    , 'privacy-browser-graphics': '选择网页 WebGL 可见图形信息预设。'
    , 'privacy-browser-webgpu': '选择是否请求阻断 WebGPU 原生适配器。'
    , 'privacy-browser-gpu-vendor': '填写网页 WebGL 厂商名称，不能修改真实硬件。'
    , 'privacy-browser-gpu-renderer': '填写网页 WebGL 渲染器名称，不能修改真实硬件。'
    , 'privacy-browser-cpu-mode': '选择网页逻辑处理器显示为原生或自定义。'
    , 'privacy-browser-cpu': '填写网页可见的逻辑处理器数，范围 1–64。'
    , 'privacy-browser-profile-select': '选择要载入的本地配置记录。'
    , 'privacy-browser-id': '只读的本地内部配置标识，不使用名称作为文件路径。'
    , 'privacy-metadata-status': '检查随包 ExifTool 组件的文件与版本状态。'
    , 'privacy-metadata-select': '主动选择一张本地图片并读取可处理字段清单，不上传文件。'
    , 'privacy-metadata-values': '只显示已选字段的实际值，结果仅留在当前页面。'
    , 'privacy-metadata-copy': '清理已选字段并写入新副本，原图不会覆盖。'
    , 'detection-refresh': '重新读取一份带有效期的本地检测快照。'
    , 'detection-export': '审阅当前检测快照后再导出，过期或历史结果不能导出。'
    , 'detection-privacy': '打开隐私保护设置，不改变当前检测结果。'
    , 'detection-browser': '打开专用浏览器与自测入口；其他浏览器仍需单独核对。'
    , 'detection-search': '只筛选当前检测快照，不修改系统设置。'
    , 'detection-audience': '按读取方筛选检测项。'
    , 'detection-category': '按检测类别筛选检测项。'
    , 'detection-filter': '按结果状态筛选检测项。'
    , 'detection-export-scope': '选择当前筛选结果或完整快照作为导出范围。'
    , 'detection-export-environment': '可选包含时区和地区字段，保存前仍可审阅。'
    , 'detection-export-observer': '可选包含观察身份与权限字段，可能增加敏感信息。'
    , 'detection-preview': '生成将要保存的脱敏报告预览，不会立即写入文件。'
    , 'detection-export-cancel': '关闭报告预览并丢弃当前未保存的导出状态。'
    , 'detection-save': '保存已经审阅的检测报告到本地。'
    , 'detection-text-preview': '生成一份更短的文字报告预览。'
    , 'detection-text-copy': '复制已经审阅的精简报告。'
    , 'privacy-engine-delete': '删除对应内核安装文件以释放空间，保留配置和浏览数据。'
  };
  el.querySelectorAll('button:not(.privacy-help), input, select, summary').forEach(control => {
    if (control.tagName === 'SUMMARY' && control.querySelector('.privacy-help')) return;
    if (control.nextElementSibling && control.nextElementSibling.classList.contains('privacy-help')) return;
    const fieldLabel = control.closest('label');
    if (fieldLabel && fieldLabel.querySelector('.privacy-help')) return;
    let text = control.getAttribute('data-help-text') || hints[control.id];
    if (!text && control.hasAttribute('data-privacy-apply')) text = '只应用这一个项目。先显示作用范围和副作用，确认后保存原值并修改；不会自动打开其他保护。';
    if (!text && control.hasAttribute('data-privacy-restore')) text = '恢复此项目记录的原值，可能重新允许信息读取；遇到外部变化时不强制覆盖。';
    if (!text && control.hasAttribute('data-privacy-settings')) text = '打开固定的 Windows 设置页面；打开页面不代表设置已经修改。';
    if (!text && control.hasAttribute('data-privacy-remove-rule')) text = '只移除这个程序的规则，程序的访问权限仍取决于其他防火墙与隧道规则。';
    if (!text && control.hasAttribute('data-detection-control')) text = '打开对应的隐私保护项目查看范围和可用操作；不会直接修改设置。';
    if (!text && control.hasAttribute('data-detection-settings')) text = '打开对应的设置引导；进入页面不会代表设置已经修改。';
    // Tabs are navigation, not actions. They do not need a help marker beside
    // every label; the selected panel and its heading already explain scope.
    if (!text && control.hasAttribute('data-privacy-tab')) return;
    if (!text && control.tagName === 'SUMMARY') text = '展开此项目的实际生效范围、副作用以及恢复方式。';
    // Do not add generic '?' markers to every input and button. They create a
    // noisy, repetitive row of icons that competes with the primary action.
    // Keep help only for controls with an explicit scope or side-effect note.
    if (!text) return;
    const card = control.closest('article,.privacy-card'), heading = card && card.querySelector('h2');
    const contextName = heading ? heading.textContent.replace(/\?/g, '').trim() + ' / ' : '';
    control.insertAdjacentHTML('afterend', privacyHelp(text, contextName + (control.getAttribute('aria-label') || control.textContent.trim() || control.id)));
  });
  el.querySelectorAll('.privacy-detect-filters > select').forEach(select => {
    const next = select.nextElementSibling, group = document.createElement('span');
    group.className = 'privacy-filter-option';
    select.parentNode.insertBefore(group, select); group.appendChild(select);
    if (next && next.classList.contains('privacy-help')) group.appendChild(next);
  });
  // A disclosure's help belongs to its summary, not its expanded content.
  // Reuse existing nodes so delegated tooltips and native markers still work.
  el.querySelectorAll('summary').forEach(summary => {
    const adjacent = summary.nextElementSibling;
    let help = summary.querySelector('.privacy-help');
    if (adjacent && adjacent.classList.contains('privacy-help')) {
      if (help) adjacent.remove();
      else help = adjacent;
    }
    if (!help) return;
    const title = summary.closest('.privacy-module-guide') && summary.querySelector(':scope > strong');
    if (title) title.insertAdjacentElement('afterend', help);
    else if (help.parentElement !== summary || help !== summary.lastElementChild) summary.appendChild(help);
  });
  // Keep each action with its help as one flex item. Inputs and segmented
  // controls retain their existing parents and their original node identity.
  el.querySelectorAll('.privacy-toolbar > button:not(.privacy-help), .privacy-actions > button:not(.privacy-help)').forEach(button => {
    const help = button.nextElementSibling;
    if (!help || !help.classList.contains('privacy-help')) return;
    const group = document.createElement('span');
    group.className = 'privacy-action-option';
    button.parentNode.insertBefore(group, button);
    group.appendChild(button); group.appendChild(help);
  });
}

// One delegated lightweight tooltip. It never takes focus or covers inputs with a modal.
(function () {
  let anchor = null, tip = null, pinned = false;
  function close() {
    if (anchor) anchor.removeAttribute('aria-describedby');
    if (tip && tip.parentNode) tip.parentNode.removeChild(tip);
    tip = null; anchor = null; pinned = false;
  }
  function show(button, pin) {
    if (!button || !button.getAttribute('data-privacy-help')) return;
    close(); anchor = button; pinned = !!pin;
    tip = document.createElement('div'); tip.id = 'privacy-tooltip'; tip.className = 'privacy-tooltip'; tip.setAttribute('role', 'tooltip'); tip.textContent = button.getAttribute('data-privacy-help');
    document.body.appendChild(tip); button.setAttribute('aria-describedby', tip.id);
    position();
  }
  function position() {
    if (!anchor || !tip || !document.documentElement.contains(anchor)) { close(); return; }
    const rect = anchor.getBoundingClientRect(), width = Math.max(120, Math.min(270, window.innerWidth - 20));
    if (rect.bottom < 0 || rect.top > window.innerHeight || rect.right < 0 || rect.left > window.innerWidth) { close(); return; }
    tip.style.width = width + 'px';
    const height = tip.getBoundingClientRect().height;
    tip.style.left = Math.max(8, Math.min(rect.left - width / 2 + rect.width / 2, window.innerWidth - width - 8)) + 'px';
    let top = Math.max(8, rect.top > height + 12 ? rect.top - height - 6 : Math.min(rect.bottom + 6, window.innerHeight - height - 8));
    const active = document.activeElement;
    if (active && /^(INPUT|TEXTAREA|SELECT)$/.test(active.tagName)) {
      const input = active.getBoundingClientRect(), left = parseFloat(tip.style.left);
      const overlaps = y => left < input.right && left + width > input.left && y < input.bottom && y + height > input.top;
      const below = Math.max(8, Math.min(rect.bottom + 6, window.innerHeight - height - 8));
      if (overlaps(top) && !overlaps(below)) top = below;
    }
    tip.style.top = top + 'px';
  }
  function button(event) { return event.target && event.target.closest ? event.target.closest('[data-privacy-help]') : null; }
  document.addEventListener('mouseover', event => { const next = button(event); if (next && next !== anchor) show(next, false); });
  document.addEventListener('mouseout', event => { if (!pinned && anchor && button(event) === anchor && !anchor.contains(event.relatedTarget)) close(); });
  document.addEventListener('focusin', event => { const next = button(event); if (next) show(next, false); else if (!pinned) close(); });
  document.addEventListener('focusout', event => { if (!pinned && button(event) === anchor) close(); });
  document.addEventListener('click', event => { const next = button(event); if (!next) { close(); return; } event.preventDefault(); if (anchor === next && pinned) close(); else show(next, true); });
  document.addEventListener('keydown', event => { if (event.key === 'Escape') close(); });
  document.addEventListener('scroll', position, true);
  window.addEventListener('resize', close);
})();

function renderPrivacyDetection(el) {
  el.classList.add('privacy');
  const model = { report:null, guides:null, snapshotId:'', expiresAt:'', stale:true, error:'', loading:false, exporting:false, audience:'', category:'', filter:'', search:'', exportOpen:false, scope:'filtered', environment:false, observer:false, preview:null, textPreview:null, previewSequence:0 };
  const context = privacyPageContext(el, 'detection', () => { model.stale=true; discardPreview(); draw(); });
  const alive=context.alive;
  const api=(method,value,extra)=>{
    if(window.__web || window.__android || !state || state.platform!=='windows' || typeof App()[method]!=='function')throw new Error('当前构建未提供此 Windows 本地接口。');
    return value===undefined?App()[method]():extra===undefined?App()[method](value):App()[method](value,extra);
  };
  const statusNames={observed:'已观察到可见信息',allowed:'配置允许读取',possible:'仍有可能读取',restricted:'检查范围内受限',unknown:'无法确认',manual:'需要手动检查'};
  const audienceNames={website:'网站',websites:'网站',web:'网站',browser:'网站',app:'应用',apps:'应用',application:'应用',native:'应用',network:'网络'};
  const findings=()=>model.report && Array.isArray(model.report.findings)?model.report.findings:[];
  const audience=item=>audienceNames[item.audience] || item.audience || '未分类';
  const formatDetectionTime=value=>{
    if(!value)return '尚未检测';
    const parsed=Date.parse(value);
    if(!Number.isFinite(parsed))return String(value);
    try{return new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(parsed));}
    catch(_){return String(value);}
  };
  const visible=()=>findings().filter(f=>(!model.audience || audience(f)===model.audience) && (!model.category || (f.category || '其他')===model.category) && (!model.search || [f.title,f.summary,f.evidence,f.scope].join(' ').toLowerCase().includes(model.search.toLowerCase())) && (!model.filter || (model.filter==='exposed'?['observed','allowed','possible'].includes(f.status):!!(f.controlId || f.settingsId))));
  const expired=()=>!!model.expiresAt && (!Number.isFinite(Date.parse(model.expiresAt)) || Date.now()>=Date.parse(model.expiresAt));
  const currentButton=button=>alive() && !!button && el.contains(button) && !model.loading && !model.exporting;
  const brief=value=>{const text=String(value || '本次没有可确认的观察。');return text.length>88?text.slice(0,87)+'…':text;};
  const canText=()=>['PrivacyDetectionTextPreview','PrivacyDetectionTextForCopy'].every(name=>Object.prototype.hasOwnProperty.call(App(),name) && typeof App()[name]==='function');
  function discardPreview(){model.preview=null;model.textPreview=null;model.previewSequence++;}
  function overview(){
    const pending=model.stale?findings().length:findings().filter(f=>!statusNames[f.status] || ['unknown','manual'].includes(f.status)).length;
    return `<div class="privacy-summary" aria-label="检测事实概览"><div class="privacy-summary-item"><strong>${findings().length}</strong><span>${model.stale?'历史观察项':'本次观察项'}</span></div><div class="privacy-summary-item${pending?' attention':''}"><strong>${pending}</strong><span>待确认项目</span></div><div class="privacy-summary-item privacy-summary-time"><strong>${esc(formatDetectionTime(model.report && model.report.generatedAt))}</strong><span>快照生成时间（本地）</span></div></div>`;
  }
  function findingCard(f){
    const source={policies:'系统策略',firewall:'程序规则',network:'服务状态',facts:'只读环境事实','browser-session':'专用浏览器管理器'}[f.source] || '本地来源';
    return `<article class="privacy-card" data-detection-finding="${esc(f.id)}"><div class="privacy-card-head"><h2>${esc(f.title || f.id)}</h2><span class="privacy-badge ${model.stale || !statusNames[f.status] || f.status==='unknown'?'attention':''}">${esc(model.stale?'历史结果，当前未确认':statusNames[f.status] || '无法确认')}</span></div><div class="privacy-scope">${esc(audience(f))} · ${esc(f.category || '其他')}</div><p>${model.stale?'上次结果：':''}${esc(brief(f.summary))}</p><details><summary>证据与范围</summary><dl>${brief(f.summary)!==f.summary?`<dt>完整观察</dt><dd>${esc(f.summary)}</dd>`:''}<dt>检查依据</dt><dd>${esc(f.evidence || '没有可验证证据。')}</dd><dt>范围</dt><dd>${esc(f.scope || '未确认')}</dd><dt>读取来源</dt><dd>${esc(source)}</dd><dt>来源读取完成</dt><dd>${esc(f.detectedAt ? formatDetectionTime(f.detectedAt) : '未提供；此项为范围说明，或来源读取未完成。')}</dd></dl></details><div class="privacy-actions">${f.controlId?`<button class="btn sm" data-detection-control="${esc(f.controlId)}">${esc(f.actionLabel || '查看可用保护')}</button>`:''}${f.settingsId?`<button class="btn sm" data-detection-settings="${esc(f.settingsId)}">${f.settingsId==='browser'?'查看专用浏览器与自测入口':'打开设置引导'}</button>`:''}</div></article>`;
  }
  function draw(){
    if(!alive())return;
    if(expired()){model.stale=true;if(model.preview || model.textPreview)discardPreview();}
    const categories=findings().map(f=>f.category || '其他').filter((c,i,all)=>all.indexOf(c)===i);
    const shown=visible().slice().sort((a,b)=>(a.status==='observed'?0:a.status==='unknown'?1:2)-(b.status==='observed'?0:b.status==='unknown'?1:2));
    el.innerHTML=`<section class="privacy-hero"><div class="privacy-eyebrow">LOCAL OBSERVATIONS · WINDOWS</div><h1>检测中心</h1><p class="muted small">查看有限观察，分清已知、配置与待确认。</p><div class="privacy-toolbar"><button class="btn sm primary" id="detection-refresh" ${model.loading || model.exporting?'disabled':''}>${model.loading?'正在检测…':'本地重新检测'}</button><button class="btn sm" id="detection-export" ${!model.report || model.stale || !model.snapshotId || model.loading || model.exporting?'disabled':''}>预览与导出</button><button class="btn sm" id="detection-privacy">隐私保护设置</button></div>${overview()}${model.stale && model.report?'<p class="privacy-note warning" role="status">历史结果，当前未确认；请重新检测后操作或导出。</p>':''}<details><summary>观察身份与检查限制</summary><p>${esc(model.report && model.report.observer || '观察身份未提供')}</p><p>管理员助手：${model.report && model.report.helperAuthorized?'已授权':'未连接或未授权'}</p><p>数量用于浏览，不是安全评分。配置值、管理状态和先前启动记录不等于当前页面或实际流量实测。</p>${(model.report && model.report.warnings || []).map(w=>`<p class="muted">${esc(w)}</p>`).join('')}</details>${model.error?`<p class="privacy-error" role="alert">${esc(model.error)}</p>`:''}</section>
    <div class="privacy-filter"><input id="detection-search" type="search" aria-label="搜索检测项" placeholder="搜索检测结果" value="${esc(model.search)}"></div><div class="privacy-detect-filters"><select id="detection-audience" aria-label="读取方"><option value="">所有读取方</option>${['网站','应用','网络'].map(a=>`<option ${model.audience===a?'selected':''}>${a}</option>`).join('')}</select><select id="detection-category" aria-label="检测类别"><option value="">所有类别</option>${categories.map(c=>`<option ${model.category===c?'selected':''}>${esc(c)}</option>`).join('')}</select><select id="detection-filter" aria-label="结果筛选"><option value="">所有结果</option><option value="exposed" ${model.filter==='exposed'?'selected':''}>只看暴露</option><option value="actionable" ${model.filter==='actionable'?'selected':''}>只看可处理</option></select></div>
    <p class="privacy-section-intro">显示 ${shown.length} 项。展开查看各自的来源、时间与范围；无法确认仍保留为未知。</p>${model.exportOpen?exportCard():''}${shown.map(findingCard).join('') || `<p class="privacy-empty">${model.loading?'正在本地检测…':model.report?'没有符合筛选条件的检测项。':'尚无报告。'}</p>`}
    <article class="privacy-card"><h2>专用浏览器与外部浏览器</h2><p>专用会话可查看管理状态与启动检查记录。其他浏览器仍需在实际配置、网站或配套扩展中自测。</p><button class="btn sm" id="detection-browser">查看专用浏览器与自测入口</button></article>${privacyModuleGuideCards(model.guides,['detection-observations'])}`;
    el.querySelector('#detection-refresh').addEventListener('click',refresh);
    el.querySelector('#detection-export').addEventListener('click',event=>{if(!currentButton(event.currentTarget) || model.stale)return;model.exportOpen=true;discardPreview();draw();});
    el.querySelector('#detection-privacy').addEventListener('click',()=>{if(alive())nav('privacy');});
    el.querySelector('#detection-browser').addEventListener('click',()=>{if(alive())nav('privacy',{tab:'browser'});});
    [['detection-audience','audience'],['detection-category','category'],['detection-filter','filter']].forEach(pair=>el.querySelector('#'+pair[0]).addEventListener('change',event=>{if(!alive() || !el.contains(event.currentTarget))return;model[pair[1]]=event.target.value;discardPreview();draw();}));
    el.querySelector('#detection-search').addEventListener('input',event=>{if(!alive() || !el.contains(event.currentTarget))return;model.search=event.target.value;const at=event.target.selectionStart;discardPreview();draw();const next=el.querySelector('#detection-search');next.focus();if(at!==null)next.setSelectionRange(at,at);});
    el.querySelectorAll('[data-detection-control]').forEach(button=>button.addEventListener('click',()=>{if(alive() && el.contains(button))nav('privacy',{controlId:button.dataset.detectionControl});}));
    el.querySelectorAll('[data-detection-settings]').forEach(button=>button.addEventListener('click',async()=>{
      if(!currentButton(button))return;
      if(button.dataset.detectionSettings==='browser'){nav('privacy',{tab:'browser'});return;}
      try{await api('PrivacyOpenSettings',button.dataset.detectionSettings);if(alive()){model.stale=true;discardPreview();draw();}}catch(error){if(alive())toast(errText(error),'err');}
    }));
    el.querySelectorAll('[data-privacy-guide-action]').forEach(button=>button.addEventListener('click',()=>{
      if(!currentButton(button))return;
      const action={kind:button.dataset.privacyGuideAction,target:button.dataset.guideTarget,label:button.textContent};
      if(privacyGuideActionAllowed(action) && action.kind==='app-page')nav(action.target);
    }));
    if(model.exportOpen){
      el.querySelector('#detection-export-scope').addEventListener('change',event=>{if(!alive() || !el.contains(event.currentTarget))return;model.scope=event.target.value;discardPreview();draw();});
      [['detection-export-environment','environment'],['detection-export-observer','observer']].forEach(pair=>el.querySelector('#'+pair[0]).addEventListener('change',event=>{if(!alive() || !el.contains(event.currentTarget))return;model[pair[1]]=event.target.checked;discardPreview();draw();}));
      el.querySelector('#detection-preview').addEventListener('click',event=>preview(event.currentTarget));
      el.querySelector('#detection-export-cancel').addEventListener('click',event=>{if(!currentButton(event.currentTarget))return;model.exportOpen=false;discardPreview();draw();});
      const save=el.querySelector('#detection-save');if(save)save.addEventListener('click',event=>savePreview(event.currentTarget));
      const text=el.querySelector('#detection-text-preview');if(text)text.addEventListener('click',event=>previewText(event.currentTarget));
      const copy=el.querySelector('#detection-text-copy');if(copy)copy.addEventListener('click',event=>copyPreviewText(event.currentTarget));
    }
    privacyDecorateHelp(el);
  }
  function exportCard(){
    return `<article class="privacy-card" id="detection-export-preview"><h2>报告预览</h2><p>选择字段，审阅后保存或复制同一份快照。默认排除时区、地区和观察身份。</p><label for="detection-export-scope">报告范围</label><select id="detection-export-scope" ${model.exporting?'disabled':''}><option value="filtered" ${model.scope==='filtered'?'selected':''}>当前筛选（${visible().length} 项）</option><option value="all" ${model.scope==='all'?'selected':''}>完整报告（${findings().length} 项）</option></select><label class="privacy-check"><input type="checkbox" id="detection-export-environment" ${model.environment?'checked':''} ${model.exporting?'disabled':''}>包含时区和地区</label><label class="privacy-check"><input type="checkbox" id="detection-export-observer" ${model.observer?'checked':''} ${model.exporting?'disabled':''}>包含观察身份与权限</label><div class="privacy-actions"><button class="btn sm" id="detection-preview" ${model.stale || model.exporting?'disabled':''}>${model.exporting?'处理中…':'生成所选字段预览'}</button><button class="btn sm" id="detection-export-cancel" ${model.exporting?'disabled':''}>关闭预览</button></div>${model.preview?`<p class="small muted">${esc(model.preview.count)} 项；有效期至 ${esc(model.preview.expiresAt)}。</p><pre class="privacy-export-json" tabindex="0" aria-label="将要保存的完整 JSON 内容">${esc(model.preview.content)}</pre><div class="privacy-actions"><button class="btn sm primary" id="detection-save" ${model.stale || model.exporting?'disabled':''}>保存这份预览</button>${canText()?`<button class="btn sm" id="detection-text-preview" ${model.stale || model.exporting?'disabled':''}>查看精简报告</button>`:''}</div>${model.textPreview?`<h3>精简报告预览</h3><pre class="privacy-text-preview" id="detection-text-content" tabindex="0" aria-label="将要复制的精简报告">${esc(model.textPreview.content)}</pre><button class="btn sm" id="detection-text-copy" ${model.stale || model.exporting?'disabled':''}>复制这份精简报告</button>`:''}`:'<p class="muted">尚未保存或复制报告。</p>'}</article>`;
  }
  async function refresh(){
    if(!alive() || model.loading || model.exporting || privacyWork.busy)return;
    model.loading=true;model.stale=true;model.error='';discardPreview();draw();
    const epoch=privacyWork.epoch;
    try{
      if(typeof App().PrivacyDetectionSnapshot!=='function')throw new Error('此构建缺少可审阅快照接口，请更新完整客户端。');
      const result=await api('PrivacyDetectionSnapshot');
      if(!alive())return;
      if(!result || !result.report || !Array.isArray(result.report.findings) || !result.snapshotId || !Number.isFinite(Date.parse(result.expiresAt)) || Date.parse(result.expiresAt)<=Date.now())throw new Error('没有返回可验证的检测快照。');
      if(epoch!==privacyWork.epoch)throw new Error('检测期间发生其他操作，请重新检测。');
      model.report=result.report;model.snapshotId=result.snapshotId;model.expiresAt=result.expiresAt;model.stale=false;
      if(Object.prototype.hasOwnProperty.call(App(),'PrivacyModuleGuides'))try{model.guides=await api('PrivacyModuleGuides');}catch(_){model.guides=null;}
      if(epoch!==privacyWork.epoch){model.stale=true;discardPreview();}
      const snapshot=model.snapshotId;
      setTimeout(()=>{if(alive() && model.snapshotId===snapshot){model.stale=true;discardPreview();draw();}},Math.max(0,Date.parse(model.expiresAt)-Date.now()));
    }catch(error){if(alive())model.error='本地检测失败：'+errText(error);}
    finally{model.loading=false;draw();}
  }
  async function preview(button){
    if(!currentButton(button) || model.stale || expired() || !model.snapshotId)return;
    const sequence=++model.previewSequence,epoch=privacyWork.epoch;
    model.exporting=true;model.error='';model.preview=null;model.textPreview=null;draw();
    try{
      const result=await api('PrivacyPreviewDetectionExport',{snapshotId:model.snapshotId,scope:model.scope,findingIds:visible().map(f=>f.id),includeEnvironment:model.environment,includeObserver:model.observer});
      if(!alive() || sequence!==model.previewSequence || epoch!==privacyWork.epoch || model.stale)return;
      if(!result || typeof result.content!=='string' || !result.previewId || !Number.isFinite(Date.parse(result.expiresAt)) || Date.parse(result.expiresAt)<=Date.now())throw new Error('未取得有效导出预览。');
      model.preview=result;
    }catch(error){if(alive() && sequence===model.previewSequence)model.error='预览未完成：'+errText(error);}
    finally{model.exporting=false;draw();}
  }
  function reviewedCurrent(preview,sequence,epoch){
    if(expired()){model.stale=true;discardPreview();draw();return false;}
    return alive() && !model.stale && sequence===model.previewSequence && epoch===privacyWork.epoch && model.preview===preview && Number.isFinite(Date.parse(preview.expiresAt)) && Date.now()<Date.parse(preview.expiresAt);
  }
  async function previewText(button){
    if(!currentButton(button) || !model.preview || !canText())return;
    const preview=model.preview,sequence=model.previewSequence,epoch=privacyWork.epoch;
    if(!reviewedCurrent(preview,sequence,epoch))return;
    model.exporting=true;model.error='';model.textPreview=null;draw();
    try{
      const result=await api('PrivacyDetectionTextPreview',preview.previewId);
      if(!reviewedCurrent(preview,sequence,epoch))return;
      if(!result || result.previewId!==preview.previewId || typeof result.content!=='string' || !result.content.length || result.content.length>1048576 || !/^[a-f0-9]{64}$/.test(result.sha256 || '') || result.count!==preview.count || !Number.isFinite(Date.parse(result.expiresAt)) || Date.parse(result.expiresAt)>Date.parse(preview.expiresAt) || Date.parse(result.expiresAt)<=Date.now())throw new Error('精简预览与已审报告不匹配。');
      model.textPreview=result;
    }catch(error){if(reviewedCurrent(preview,sequence,epoch))model.error='精简预览未完成：'+errText(error);}
    finally{model.exporting=false;draw();}
  }
  async function copyPreviewText(button){
    if(!currentButton(button) || !model.preview || !model.textPreview)return;
    const preview=model.preview,text=model.textPreview,sequence=model.previewSequence,epoch=privacyWork.epoch;
    const content=el.querySelector('#detection-text-content');
    if(!reviewedCurrent(preview,sequence,epoch) || Date.now()>=Date.parse(text.expiresAt))return;
    if(!content || content.textContent!==text.content){model.error='复制未完成：可见预览已改变，请重新审阅。';draw();return;}
    model.exporting=true;model.error='';draw();
    try{
      const result=await api('PrivacyDetectionTextForCopy',text.previewId,text.sha256);
      if(!reviewedCurrent(preview,sequence,epoch) || model.textPreview!==text || Date.now()>=Date.parse(text.expiresAt))return;
      const current=el.querySelector('#detection-text-content');
      if(result!==text.content || !current || current.textContent!==text.content)throw new Error('返回内容与可见预览不同，未复制。');
      const copied=await copyText(result);
      if(reviewedCurrent(preview,sequence,epoch)){if(!copied)throw new Error('剪贴板没有确认写入。');toast('已复制审阅的同一份精简报告。');}
    }catch(error){if(reviewedCurrent(preview,sequence,epoch))model.error='复制未完成：'+errText(error);}
    finally{model.exporting=false;draw();}
  }
  async function savePreview(button){
    if(!currentButton(button) || !model.preview)return;
    const preview=model.preview,sequence=model.previewSequence,epoch=privacyWork.epoch;
    if(!reviewedCurrent(preview,sequence,epoch))return;
    model.exporting=true;model.error='';draw();
    try{const saved=await api('PrivacyExportDetection',preview.previewId);if(reviewedCurrent(preview,sequence,epoch)){toast(saved?'已保存审阅的同一份快照。分享前仍请核对上下文。':'已取消保存。');if(saved)discardPreview();}}
    catch(error){if(reviewedCurrent(preview,sequence,epoch)){model.error='保存未完成：'+errText(error);}}
    finally{model.exporting=false;draw();}
  }
  draw();refresh();
}
