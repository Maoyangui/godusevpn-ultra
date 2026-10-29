(function (root) {
  'use strict';
  const catalog = [
    ['webrtc', 'network.webRTCIPHandlingPolicy', 'disable_non_proxied_udp', '限制 WebRTC 非代理 UDP', '可能影响通话质量。此设置限制路径，不会完全禁用 WebRTC，也不能替代隧道防直连。'],
    ['prediction', 'network.networkPredictionEnabled', false, '关闭浏览器预测加载', '减少浏览器主动预连接和预加载；网页自己发起的预连接与 DNS 预取仍可能存在。'],
    ['suggest', 'services.searchSuggestEnabled', false, '关闭地址栏云搜索建议', '不再把输入内容用于该云建议服务，地址栏建议可能减少；搜索提交仍会发送给搜索引擎。'],
    ['spelling', 'services.spellingServiceEnabled', false, '关闭云拼写服务', '减少将输入文本发送至云拼写服务，本地拼写能力仍可能可用。'],
    ['alternate', 'services.alternateErrorPagesEnabled', false, '关闭云错误页面辅助', '减少网络错误时联系浏览器厂商服务，错误页面辅助可能减少。'],
    ['pings', 'websites.hyperlinkAuditingEnabled', false, '关闭链接审计 ping', '阻止链接的 ping 审计，不会阻止网页通过其他请求发送跟踪信息。'],
    ['referrer', 'websites.referrersEnabled', false, '关闭 Referer 发送', '减少来源页面暴露；可能破坏防盗链、下载和部分站点验证。'],
    ['thirdparty', 'websites.thirdPartyCookiesAllowed', false, '限制第三方 Cookie', '可能影响嵌入登录。浏览器豁免与 Storage Access API 等机制仍可能允许特定站点访问。'],
    ['topics', 'websites.topicsEnabled', false, '关闭广告 Topics', '只关闭此受支持的广告接口，不阻止站点自己的用户画像。'],
    ['fledge', 'websites.fledgeEnabled', false, '关闭兴趣组广告接口', '浏览器可能已移除此接口；不可用时显示不支持，不伪装为已保护。'],
    ['measurement', 'websites.adMeasurementEnabled', false, '关闭广告衡量接口', '只影响此浏览器公开的广告衡量机制，不阻止所有广告请求。']
  ].map(row => ({ id: row[0], path: row[1], desired: row[2], title: row[3], impact: row[4], type: 'privacy' }));
  [
    ['camera', '摄像头', '拒绝时会阻止视频会议和拍摄。'],
    ['microphone', '麦克风', '拒绝时会阻止语音会议和录音。'],
    ['location', '网页定位', '网页仍可通过出口 IP、账号和用户输入推测位置。'],
    ['notifications', '网页通知', '拒绝时会阻止站点提醒。'],
    ['clipboard', '剪贴板读取', '浏览器若未提供此接口，将显示不支持。用户主动粘贴仍能向网页提供文本。'],
    ['javascript', '网页 JavaScript', '拒绝时会让多数现代网页失效，仅适合用户明确选择的严格浏览场景。'],
    ['automaticDownloads', '自动多文件下载', '拒绝时可能阻止网页连续下载多个文件。']
  ].forEach(row => catalog.push({ id: 'content-' + row[0], path: row[0], desired: 'block', values: row[0] === 'javascript' ? ['allow', 'block'] : ['allow', 'ask', 'block'], allowRequiresOrigin: row[0] === 'camera' || row[0] === 'microphone', title: row[1] + '权限', impact: row[2] + ' 仅当前正常浏览配置；站点例外或更高优先级策略可能不同。更换来源会替换本扩展此类规则。', type: 'content' }));

  function call(api, owner, method, details) {
    return new Promise((resolve, reject) => {
      try { owner[method](details, result => { const err = api.runtime.lastError; if (err) reject(new Error(err.message)); else resolve(result); }); }
      catch (error) { reject(error); }
    });
  }
  function target(api, item) {
    if (item.type === 'content') return api.contentSettings && api.contentSettings[item.path];
    return item.path.split('.').reduce((value, key) => value && value[key], api.privacy);
  }
  function writable(info) { return info.levelOfControl === 'controllable_by_this_extension' || info.levelOfControl === 'controlled_by_this_extension'; }
  class Engine {
    constructor(api) { this.api = api; this.busy = false; }
    async journal() { const result = await call(this.api, this.api.storage.local, 'get', ['managed']); return result.managed || {}; }
    async save(managed) { await call(this.api, this.api.storage.local, 'set', { managed: managed }); }
    async read(item, origin) {
      const setting = target(this.api, item);
      if (!setting || typeof setting.get !== 'function') return { supported: false, detail: '当前浏览器未提供此接口。' };
      const data = await call(this.api, setting, 'get', item.type === 'content' ? { primaryUrl: origin || 'https://example.invalid/', secondaryUrl: origin || 'https://example.invalid/', incognito: false } : { incognito: false });
      return { supported: true, value: item.type === 'content' ? data.setting : data.value, control: data.levelOfControl || '', writable: item.type === 'content' ? true : writable(data), sample: item.type === 'content' };
    }
    async report() {
      const managed = await this.journal();
      return Promise.all(catalog.map(async item => {
        const setting = target(this.api, item), clearable = !!setting && typeof setting.clear === 'function';
        try { const record = managed[item.id], info = await this.read(item, record && record.origin); return Object.assign({}, item, info, { clearable: clearable, managed: !!record, appliedValue: record && record.desired, origin: record && record.origin || '', stage: record && record.stage, orphaned: !record && info.control === 'controlled_by_this_extension', checkedAt: new Date().toISOString() }); }
        catch (error) { return Object.assign({}, item, { supported: false, unknown: true, clearable: clearable, managed: !!managed[item.id], detail: error.message, checkedAt: new Date().toISOString() }); }
      }));
    }
    async apply(id, options) {
      if (this.busy) throw new Error('另一项操作尚未完成。');
      const item = catalog.find(entry => entry.id === id);
      if (!item) throw new Error('未知设置。');
      this.busy = true;
      try {
        const desired = item.type === 'content' && options && options.value !== undefined ? options.value : item.desired;
        if (item.type === 'content' && item.values.indexOf(desired) < 0) throw new Error('此权限 API 不支持所选模式。');
        let origin = '';
        if (item.type === 'content' && options && options.origin) {
          if (typeof options.origin !== 'string' || options.origin.length > 2048) throw new Error('来源长度或类型无效。');
          const url = new URL(options.origin);
          if (url.protocol !== 'https:' || url.username || url.password || url.pathname !== '/' || url.search || url.hash || url.hostname.indexOf('*') >= 0) throw new Error('来源必须是明确 HTTPS 站点原点，不能包含路径、凭据或通配符。');
          origin = url.origin;
        }
        if (item.allowRequiresOrigin && desired === 'allow' && !origin) throw new Error('浏览器禁止摄像头/麦克风的全站允许；请填写明确 HTTPS 站点来源。');
        const before = await this.read(item, origin);
        if (!before.supported || !before.writable) throw new Error('此设置不支持修改，或正受企业策略、其他扩展控制。');
        const managed = await this.journal();
        if (managed[id] && managed[id].stage && managed[id].stage !== 'active') throw new Error('上次操作未完成；请先撤销本扩展覆盖并重新检查。');
        // Persist only settings ownership, never browsing or fingerprint data.
        const original = managed[id] ? managed[id].original : before.value;
        managed[id] = { original: original, appliedAt: new Date().toISOString(), type: item.type, stage: 'applying', desired: desired, origin: origin };
        await this.save(managed);
        if (item.type === 'content') await call(this.api, target(this.api, item), 'clear', { scope: 'regular' });
        await call(this.api, target(this.api, item), 'set', item.type === 'content' ? { primaryPattern: origin ? origin + '/*' : '<all_urls>', setting: desired, scope: 'regular' } : { value: desired, scope: 'regular' });
        const after = await this.read(item, origin);
        if (after.value !== desired || (item.type === 'privacy' && after.control !== 'controlled_by_this_extension')) throw new Error('设置调用已返回，但读回值或控制权不符合目标。请检查策略，必要时撤销本扩展覆盖。');
        managed[id].stage = 'active'; await this.save(managed);
        return after;
      } finally { this.busy = false; }
    }
    async restore(id) {
      if (this.busy) throw new Error('另一项操作尚未完成。');
      const item = catalog.find(entry => entry.id === id);
      if (!item) throw new Error('未知设置。');
      this.busy = true;
      try {
        const managed = await this.journal();
        let before;
        try { before = await this.read(item, managed[id] && managed[id].origin); }
        catch (error) { if (!managed[id] && item.type !== 'content') throw error; before = {}; }
        if (!managed[id] && item.type !== 'content' && before.control !== 'controlled_by_this_extension') throw new Error('没有发现本扩展覆盖；不会改写底层配置。');
        // Clear this extension's layer. Do not write a historical value over a user's newer setting.
        const setting = target(this.api, item);
        if (!setting) throw new Error('浏览器已移除此接口。可停用或移除扩展来撤销其覆盖。');
        if (managed[id]) { managed[id].stage = 'restoring'; await this.save(managed); }
        await call(this.api, setting, 'clear', { scope: 'regular' });
        const after = await this.read(item);
        if (item.type === 'privacy' && after.control === 'controlled_by_this_extension') throw new Error('浏览器仍报告本扩展控制，保留恢复记录。');
        delete managed[id]; await this.save(managed);
        return Object.assign(after, { clearAcknowledged: true, verified: item.type === 'privacy' });
      } finally { this.busy = false; }
    }
  }
  class Coordinator {
    constructor(engine) { this.engine = engine; this.queue = Promise.resolve(); }
    request(action, id, options) {
      if (['report', 'apply', 'restore'].indexOf(action) < 0) return Promise.reject(new Error('未知操作。'));
      const run = () => this.engine[action](id, options);
      const result = this.queue.then(run, run);
      this.queue = result.catch(() => {});
      return result;
    }
  }
  class Client {
    constructor(api) { this.api = api; }
    request(action, id, options) {
      return new Promise((resolve, reject) => this.api.runtime.sendMessage({ type: 'ultra_privacy_v1', action: action, id: id, options: options }, reply => {
        const err = this.api.runtime.lastError;
        if (err || !reply || !reply.ok) reject(new Error(err && err.message || reply && reply.error || '后台操作结果未知；请重新检查，不自动重试。'));
        else resolve(reply.result);
      }));
    }
    report() { return this.request('report'); }
    apply(id, options) { return this.request('apply', id, options); }
    restore(id) { return this.request('restore', id); }
  }
  const exported = { catalog: catalog, Engine: Engine, Coordinator: Coordinator, Client: Client };
  if (typeof module !== 'undefined' && module.exports) module.exports = exported;
  else root.UltraPrivacy = exported;
})(typeof self !== 'undefined' ? self : this);
