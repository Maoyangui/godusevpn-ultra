(function (root) {
  'use strict';
  const known = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content', 'utm_id', 'gclid', 'dclid', 'fbclid', 'msclkid', 'mc_cid', 'mc_eid'];
  function cleanLink(text) {
    let url;
    try { url = new URL(String(text).trim()); } catch (error) { throw new Error('请输入完整的 http 或 https 链接。'); }
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) throw new Error('只处理不含用户名和密码的 http / https 链接。');
    const keys = Array.from(url.searchParams.keys());
    // Conservative exclusions. Manual cleaning still requires user review before copying.
    if (/(?:^|[\/._-])(login|signin|oauth|auth|callback|checkout|payment|pay|order|sso|verify|reset)(?:[\/._-]|$)/i.test(url.pathname) || keys.some(key => /^(?:sig|signature|x-amz-.+|x-goog-.+|token|access_token|auth|authorization|code|state|nonce|hmac|hash|expires|policy|key-pair-id)$/i.test(key))) {
      return { url: String(text).trim(), removed: [], skipped: true, reason: '检测到登录、支付、订单或签名参数，为避免破坏验证，保留原链接。' };
    }
    const removed = [];
    keys.forEach(key => { if (known.includes(key.toLowerCase())) { url.searchParams.delete(key); removed.push(key); } });
    // Do not normalize query encoding at all when nothing is removed.
    return { url: removed.length ? url.href : String(text).trim(), removed: removed, skipped: false, reason: removed.length ? '仅移除了已知跟踪参数。请检查结果，再手动复制使用。' : '没有发现可清理的已知跟踪参数。' };
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = { cleanLink: cleanLink };
  else root.UltraCleanLink = cleanLink;
})(typeof self !== 'undefined' ? self : this);
