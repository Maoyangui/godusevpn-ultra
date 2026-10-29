// Current-user country/region UI. No network, clipboard, tunnel or helper API.
(function (global) {
  'use strict';
  const mounts = new WeakMap();
  const codeOK = value => typeof value === 'string' && /^(?:[A-Z]{2}|[0-9]{3})$/.test(value);
  const escape = value => String(value == null ? '' : value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const scope = '仅当前 Windows 用户的国家或地区；不更改 GPS、IP、显示语言、键盘、时区或浏览器环境。';
  const competition = '其他程序同时修改仍有竞争窗口；发现不同状态会停止，不强制恢复。';
  const names = {available:'尚未修改',configured:'已读回当前地区',recovery_pending:'待恢复核对',external_change:'外部更改已保留',backup_error:'备份不可确认',unknown:'当前值未确认',unsupported:'当前用户不可用'};
  const help = {
    scope:'只修改当前普通 Windows 用户的国家/地区设置，会影响该用户的其他应用。它不是 GPS 模拟或 GeoIP，更不会调整显示语言、键盘、日期格式、时区或浏览器参数。无需管理员助手；提升或替代管理员身份会被后端拒绝。',
    choice:'选项只来自当前 Windows 支持清单。选择仅保存本页草稿，不修改系统，也不会根据代理出口自动选择。',
    apply:'确认后再次检查当前地区；后端先持久保存原值，再修改并独立读取。其他应用可能缓存旧值，需要重新打开后核对。',
    restore:'只恢复本应用保存的最初地区。备份损坏、身份不符或观察到外部更改时停止，不覆盖其他来源的新值。Windows 接口没有原子比较替换，无法识别其他程序先改走再改回相同值。',
    refresh:'只重新读取当前用户国家/地区、系统清单与备份状态，不应用草稿、不修改系统。'
  };
  function errorText(error) {
    const text = String(error && error.message || error || '');
    const codes = {
      REGION_CURRENT_ORDINARY_USER_UNAVAILABLE:'当前普通用户或 Windows 接口不可用；未改用管理员账户。',
      REGION_CODE_NOT_IN_SYSTEM_CATALOG:'所选代码不在当前系统清单，请重新读取后选择。',
      REGION_PRIVATE_BACKUP_UNAVAILABLE:'地区备份不可确认，已停止；保留恢复记录。',
      REGION_OPERATION_BUSY:'另一个地区操作正在处理，请稍后重新检查。',
      REGION_EXTERNAL_CHANGE_PRESERVED:'检测到外部更改，保留当前地区，没有强制覆盖。',
      REGION_RECOVERY_REQUIRED:'上次操作未完成，请检查并恢复原地区后再应用。',
      REGION_CHANGE_NOT_CONFIRMED:'修改结果未完整确认；保留恢复线索，请重新检查。'
    };
    for (const code of Object.keys(codes)) if (text.includes(code)) return codes[code];
    return '操作未完成，请重新读取当前状态。没有改用其他账户或自动重试。';
  }
  function checkedReport(value) {
    if (!value || typeof value !== 'object' || typeof value.supported !== 'boolean' || !Object.prototype.hasOwnProperty.call(names,value.status) || !Array.isArray(value.choices) || value.choices.length > 1024 || value.atomicExternalCAS !== false) throw Error('invalid region report');
    const seen = new Set(), choices = value.choices.map(choice => {
      if (!choice || !codeOK(choice.code) || seen.has(choice.code) || typeof choice.label !== 'string' || !choice.label || choice.label.length > 512 || /[\x00-\x1f\x7f-\x9f\u200b-\u200f\u202a-\u202e\u2066-\u2069]/.test(choice.label)) throw Error('invalid region choices');
      seen.add(choice.code); return {code:choice.code,label:choice.label};
    });
    if (value.currentCode && !codeOK(value.currentCode) || value.originalCode && !codeOK(value.originalCode)) throw Error('invalid region identity');
    const observed = typeof value.observedAt === 'string' && Number.isFinite(Date.parse(value.observedAt));
    return {supported:value.supported,status:value.status,choices,currentCode:value.currentCode || '',originalCode:value.originalCode || '',managed:value.managed === true,canApply:value.canApply === true && observed,canRestore:value.canRestore === true && observed,observedAt:observed ? value.observedAt : '',detail:typeof value.detail === 'string' ? value.detail.slice(0,2048) : ''};
  }
  const fingerprint = report => JSON.stringify([report.supported,report.currentCode,report.status,report.managed,report.originalCode,report.canApply,report.canRestore]);
  function mount(facade, slot) {
    if (!slot || typeof slot.querySelector !== 'function') return function () {};
    const previous = mounts.get(slot); if (previous) previous();
    const draft = facade && facade.draft && typeof facade.draft === 'object' ? facade.draft : {code:'',touched:false};
    const model = {report:null,error:'',message:'',pending:'',help:'',readable:false};
    let closed = false, generation = 0;
    const current = () => !closed && slot.isConnected && mounts.get(slot) === cleanup && facade && typeof facade.alive === 'function' && facade.alive();
    const externalBusy = () => typeof facade.busy === 'function' && facade.busy();
    const currentNode = node => current() && node && slot.contains(node);
    function cleanup() {
      if (closed) return;
      closed = true; generation++;
      if (mounts.get(slot) === cleanup) {
        mounts.delete(slot);
        slot.querySelectorAll('button,select').forEach(node => { node.disabled = true; });
      }
    }
    mounts.set(slot,cleanup);
    const choice = (report, code) => report && report.choices.find(item => item.code === code);
    function canApply(report) {
      return !!(model.readable && report && report.supported && report.canApply && ['available','configured'].includes(report.status) && choice(report,report.currentCode) && choice(report,draft.code) && draft.code !== report.currentCode);
    }
    function canRestore(report) {
      return !!(model.readable && report && report.supported && report.managed && report.canRestore && ['configured','recovery_pending'].includes(report.status) && codeOK(report.currentCode) && choice(report,report.originalCode));
    }
    const standardHelp = () => typeof facade.help === 'function';
    const question = (key, label) => standardHelp() ? facade.help(help[key],label) : `<button type="button" class="privacy-help" data-region-help="${key}" aria-label="${escape(label)}：帮助" title="${escape(help[key])}" ${model.pending ? 'disabled' : ''}>?</button>`;
    function draw() {
      if (!current()) return;
      const report = model.report, busy = !!model.pending || externalBusy(), choices = report ? report.choices : [];
      const selected = choice(report,draft.code), actual = choice(report,report && report.currentCode);
      const badge = model.pending === 'read' ? '正在读取…' : model.pending === 'confirm' ? '等待确认' : model.pending ? '正在核对操作…' : model.readable && report ? names[report.status] : '尚未确认';
      slot.innerHTML = `<article class="privacy-card privacy-form" id="privacy-region-card" aria-busy="${!!model.pending}"><div class="privacy-card-head"><h2>当前用户国家或地区${question('scope','国家或地区范围')}</h2><span class="privacy-badge">${escape(badge)}</span></div><p>${scope}</p>${report && report.currentCode ? `<dl><dt>本次读回</dt><dd>${escape(actual ? actual.label : report.currentCode)}</dd>${report.originalCode ? `<dt>本应用保存的原值</dt><dd>${escape(choice(report,report.originalCode) ? choice(report,report.originalCode).label : report.originalCode)}</dd>` : ''}${report.observedAt ? `<dt>读取时间（UTC）</dt><dd>${escape(report.observedAt)}</dd>` : ''}</dl>` : ''}${report && report.detail ? `<p class="muted small">${escape(report.detail)}</p>` : ''}<label for="privacy-region-choice" class="privacy-time-label">选择 Windows 国家或地区${question('choice','选择国家或地区')}</label><select id="privacy-region-choice" aria-label="Windows 国家或地区" ${busy || !model.readable || !report || !report.supported || !choices.length ? 'disabled' : ''}><option value="" ${!selected ? 'selected' : ''}>${choices.length ? '从系统清单选择' : '尚无可验证的系统清单'}</option>${choices.map(item => `<option value="${item.code}" ${draft.code === item.code ? 'selected' : ''}>${escape(item.label)}</option>`).join('')}</select>${selected && report ? `<p class="privacy-pending">${draft.code === report.currentCode ? '当前选择与读回一致；没有待提交的修改。' : '待应用：仅保存了本页选择，系统尚未修改。'}</p>` : ''}${model.error ? `<p class="privacy-error" role="alert">${escape(model.error)}</p>` : ''}${model.message ? `<p class="privacy-action-hint" role="status">${escape(model.message)}</p>` : ''}<div class="privacy-actions"><button type="button" class="btn sm primary" id="privacy-region-apply" ${busy || !canApply(report) ? 'disabled' : ''}>应用所选地区</button>${question('apply','应用所选地区')}<button type="button" class="btn sm" id="privacy-region-refresh" ${busy ? 'disabled' : ''}>重新读取</button>${question('refresh','重新读取地区')}<button type="button" class="btn sm" id="privacy-region-restore" ${busy || !canRestore(report) ? 'disabled' : ''}>恢复原地区</button>${question('restore','恢复原地区')}</div><details class="privacy-note"><summary>影响、恢复与同时修改${question('restore','影响与恢复')}</summary><p>这会影响当前用户的其他应用；已打开的应用可能缓存旧值，需要重新打开后核对。原值由本应用单独保存，恢复不包含其他系统策略。</p><p>${competition} Windows 接口没有原子比较替换，也无法识别外部先改走再改回相同值。</p><p>显示语言、日期与数字格式、键盘编辑仍须使用各自系统设置，未随本操作更改。</p></details>${model.help ? `<p class="privacy-note" role="note" id="privacy-region-help">${escape(help[model.help])}</p>` : ''}</article>`;
      const select = slot.querySelector('#privacy-region-choice');
      select.addEventListener('change', () => {
        if (!currentNode(select) || model.pending || externalBusy() || !choice(model.report,select.value)) return;
        draft.code = select.value; draft.touched = true; model.error = ''; model.message = ''; generation++; draw();
      });
      const refresh = slot.querySelector('#privacy-region-refresh');
      refresh.addEventListener('click', () => { if (currentNode(refresh) && !model.pending && !externalBusy()) read(); });
      for (const kind of ['apply','restore']) {
        const button = slot.querySelector('#privacy-region-' + kind);
        button.addEventListener('click', () => { if (currentNode(button)) change(kind); });
      }
      slot.querySelectorAll('[data-region-help]').forEach(button => button.addEventListener('click', event => {
        event.preventDefault();
        if (!currentNode(button) || model.pending) return;
        model.help = button.getAttribute('data-region-help'); draw();
      }));
      if (typeof facade.decorate === 'function') facade.decorate();
    }
    function accept(report) {
      model.report = report; model.readable = true;
      if (draft.touched !== true) draft.code = choice(report,report.currentCode) ? report.currentCode : '';
    }
    async function read() {
      if (!current() || model.pending) return;
      const token = ++generation; model.pending = 'read'; model.readable = false; model.report = null; model.error = ''; model.message = ''; draw();
      try {
        const value = checkedReport(await facade.Report());
        if (!current() || token !== generation) return;
        accept(value);
      } catch (error) {
        if (!current() || token !== generation) return;
        model.error = errorText(error); model.report = null; model.readable = false;
      } finally { if (current() && token === generation) { model.pending = ''; draw(); } }
    }
    async function change(kind) {
      const before = model.report;
      if (!current() || model.pending || externalBusy() || !(kind === 'apply' ? canApply(before) : canRestore(before))) return;
      const code = kind === 'apply' ? draft.code : before.originalCode, selected = choice(before,code), key = fingerprint(before), token = ++generation;
      model.pending = 'confirm'; model.error = ''; model.message = ''; draw();
      let written = false, resultMessage = '', writeStatus = '';
      try {
        const approved = await facade.confirm((kind === 'apply' ? '将当前用户国家或地区改为“' + selected.label + '”？' : '恢复本应用保存的原地区“' + selected.label + '”？') + '\n' + scope + '\n' + competition, {ok:kind === 'apply' ? '应用并读回' : '恢复并读回',danger:false});
        if (!current() || token !== generation) return;
        if (!approved) return;
        if (externalBusy() || kind === 'apply' && draft.code !== code) throw Error('confirmation invalidated');
        model.pending = 'preflight'; draw();
        const fresh = checkedReport(await facade.Report());
        if (!current() || token !== generation) return;
        accept(fresh);
        if (fingerprint(fresh) !== key || !choice(fresh,code) || choice(fresh,code).label !== selected.label || externalBusy() || !(kind === 'apply' ? canApply(fresh) : canRestore(fresh))) {
          model.error = '确认期间地区或恢复状态已改变，未提交修改。请查看最新值后重新确认。'; return;
        }
        model.pending = 'write'; model.readable = false; draw();
        written = true;
        const result = await (kind === 'apply' ? facade.Apply(code) : facade.Restore());
        if (!current() || token !== generation) return;
        if (!result || result.verified !== true || result.currentCode !== code || typeof result.changed !== 'boolean' || ![kind === 'apply' ? 'applied' : 'restored','unchanged'].includes(result.status) || result.status === 'unchanged' && result.changed) throw Error('unconfirmed result');
        writeStatus = result.status;
        resultMessage = result.status === 'unchanged' ? '当前值已读回一致，没有再次写入。' : kind === 'apply' ? '已应用并读回当前用户地区。' : '原地区已恢复并读回。';
      } catch (error) {
        if (!current() || token !== generation) return;
        model.error = errorText(error);
      } finally {
        if (current() && token === generation) {
          if (written) {
            model.readable = false;
            try {
              const after = checkedReport(await facade.Report());
              if (!current() || token !== generation) return;
              accept(after);
              const ownershipConfirmed = writeStatus === 'unchanged' || (kind === 'apply' ? after.status === 'configured' && after.managed : after.status === 'available' && !after.managed);
              if (resultMessage && after.currentCode === code && after.observedAt && ['available','configured'].includes(after.status) && ownershipConfirmed) model.message = resultMessage;
              else if (resultMessage) model.error = '操作后的地区再次改变或无法确认，请检查最新读回；没有自动重试。';
            } catch (_) { if (current() && token === generation) { model.report = null; model.readable = false; model.error = '操作后重新读取失败，结果尚未确认。请重新检查；恢复记录由后端保留。'; } }
          }
          if (current() && token === generation) { model.pending = ''; draw(); }
        }
      }
    }
    if (!facade || ['Report','Apply','Restore','alive','confirm'].some(name => typeof facade[name] !== 'function')) {
      model.error = '当前客户端没有完整的地区编辑接口，请使用完整 Ultra Windows 客户端。'; draw();
    } else read();
    return cleanup;
  }
  function unmount(slot) { const cleanup = mounts.get(slot); if (cleanup) cleanup(); }
  global.PrivacyRegionUI = Object.freeze({mount,unmount});
})(window);
