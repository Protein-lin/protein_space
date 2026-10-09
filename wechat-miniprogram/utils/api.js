const { SSEParser } = require('./sse');
function normalizeBase(value) {
  const base = value.trim().replace(/\/+$/, '');
  if (!/^https:\/\/[a-z0-9.-]+(?::\d+)?$/i.test(base)) throw new Error('请输入 HTTPS 站点地址，例如 https://linpro.top（不要加 /api）');
  return base;
}
function headers(key) { return { 'Content-Type': 'application/json', ...(key ? { 'X-API-Key': key } : {}) }; }
function request(base, key, path) {
  return new Promise((resolve, reject) => wx.request({
    url: base + '/api' + path, header: headers(key), timeout: 20000,
    success: r => r.statusCode >= 200 && r.statusCode < 300 ? resolve(r.data) : reject(new Error(`HTTP ${r.statusCode}：${typeof r.data === 'string' ? r.data : JSON.stringify(r.data)}`)),
    fail: e => reject(new Error(e.errMsg || '网络连接失败'))
  }));
}
function stream(base, key, body, onDelta, onFinish) {
  let settled = false, received = false, status = 200;
  const parser = new SSEParser(onDelta);
  const finish = error => { if (!settled) { settled = true; onFinish(error); } };
  const task = wx.request({
    url: base + '/api/chat/stream', method: 'POST', header: { ...headers(key), Accept: 'text/event-stream' },
    data: body, enableChunked: true, responseType: 'arraybuffer', timeout: 600000,
    success(r) {
      if (settled) return;
      try {
        if (r.statusCode < 200 || r.statusCode >= 300) throw new Error(`HTTP ${r.statusCode}，请检查 API Key、模型权限及服务状态`);
        if (!received && r.data) parser.push(r.data);
        parser.finish(); finish();
      } catch (e) { finish(e); }
    },
    fail: e => finish(new Error(e.errMsg || '网络连接失败'))
  });
  task.onHeadersReceived(r => { status = r.statusCode; });
  task.onChunkReceived(r => {
    if (settled || status < 200 || status >= 300) return;
    try { received = true; parser.push(r.data); } catch (e) { finish(e); task.abort(); }
  });
  return { abort() { finish(new Error('已停止生成，回复可能不完整')); task.abort(); } };
}
module.exports = { normalizeBase, request, stream };
