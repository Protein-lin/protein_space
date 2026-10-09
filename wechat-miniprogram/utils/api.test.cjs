const { test } = require('node:test');
const assert = require('node:assert/strict');
const { stream, request } = require('./api');
function mock() {
  let options, chunk, headers, aborted = false;
  global.wx = { request(o) { options = o; return {
    onHeadersReceived(fn) { headers = fn; }, onChunkReceived(fn) { chunk = fn; },
    abort() { aborted = true; options.fail({ errMsg: 'request:fail abort' }); }
  }; } };
  return { get options() { return options; }, get aborted() { return aborted; },
    headers(code) { headers({ statusCode: code }); },
    chunk(text) { chunk({ data: new TextEncoder().encode(text).buffer }); }
  };
}
test('stream sends authenticated context and finishes only once', () => {
  const m = mock(), body = { conversation_id: 'test', model: 'model', messages: [{ role: 'user', content: 'hi' }] };
  let text = '', finishes = 0;
  stream('https://example.com', 'personal-key', body, d => text += d, e => { assert.equal(e, undefined); finishes++; });
  assert.equal(m.options.header['X-API-Key'], 'personal-key');
  assert.deepEqual(m.options.data, body);
  m.headers(200); m.chunk('data: {"delta":"回复"}\n\ndata: [DONE]\n\n');
  m.options.success({ statusCode: 200 }); m.options.fail({ errMsg: 'late' });
  assert.equal(text, '回复'); assert.equal(finishes, 1);
});
test('abort ignores subsequent chunks and reports one cancellation', () => {
  const m = mock(); let count = 0, text = '';
  const task = stream('https://example.com', '', {}, d => text += d, e => { assert.match(e.message, /停止/); count++; });
  task.abort(); m.chunk('data: {"delta":"late"}\n\n');
  assert.equal(m.aborted, true); assert.equal(count, 1); assert.equal(text, '');
});
test('HTTP auth failure does not decode the response as SSE', () => {
  const m = mock(); let error;
  stream('https://example.com', 'bad', {}, () => assert.fail(), e => error = e);
  m.headers(401); m.chunk('unauthorized'); m.options.success({ statusCode: 401 });
  assert.match(error.message, /401/);
});
test('non-chunked fallback handles completed response', () => {
  const m = mock(); let text = '', error;
  stream('https://example.com', '', {}, d => text += d, e => error = e);
  m.options.success({ statusCode: 200, data: new TextEncoder().encode('data: {"content":"你好"}\n\ndata: [DONE]\n\n').buffer });
  assert.equal(error, undefined); assert.equal(text, '你好');
});
test('model request rejects non-success status', async () => {
  const m = mock(); const result = request('https://example.com', 'key', '/models');
  m.options.success({ statusCode: 403, data: 'denied' });
  await assert.rejects(result, /403/);
});
