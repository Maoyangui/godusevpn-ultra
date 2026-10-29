'use strict';
importScripts('core.js');
const coordinator = new UltraPrivacy.Coordinator(new UltraPrivacy.Engine(chrome));
chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (!message || message.type !== 'ultra_privacy_v1') return false;
  if (!sender || sender.id !== chrome.runtime.id || !sender.url || sender.url.indexOf(chrome.runtime.getURL('')) !== 0) { respond({ ok: false, error: '仅接受本扩展内部页面操作。' }); return false; }
  coordinator.request(message.action, message.id, message.options).then(result => respond({ ok: true, result: result }), error => respond({ ok: false, error: error.message || '后台操作未完成。' }));
  return true;
});
