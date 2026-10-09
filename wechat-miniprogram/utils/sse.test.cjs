const { test } = require('node:test');
const assert = require('node:assert/strict');
const { SSEParser } = require('./sse');
const { normalizeBase } = require('./api');
function bytes(text) { return new TextEncoder().encode(text); }
test('every possible UTF-8 split preserves Chinese and emoji, CRLF and multiple events', () => {
  const wire = bytes(': heartbeat\r\n\r\ndata: {"delta":"你好🌏"}\r\n\r\ndata: {"content":"！"}\n\ndata: [DONE]\n\n');
  for (let split = 0; split <= wire.length; split++) {
    let output = ''; const parser = new SSEParser(d => output += d);
    parser.push(wire.slice(0, split)); parser.push(wire.slice(split)); parser.finish();
    assert.equal(output, '你好🌏！');
  }
});
test('single byte chunks and final event without newline', () => {
  let output = ''; const parser = new SSEParser(d => output += d);
  for (const byte of bytes('data: {"delta":"中文"}\n\ndata: [DONE]')) parser.push(Uint8Array.of(byte));
  parser.finish(); assert.equal(output, '中文');
});
test('upstream errors and truncated responses are surfaced', () => {
  assert.throws(() => new SSEParser(() => {}).push(bytes('data: {"error":"denied"}\n\n')), /denied/);
  assert.throws(() => new SSEParser(() => {}).finish(), /提前结束/);
  assert.throws(() => new SSEParser(() => {}).push(bytes('data: nope\n\n')), SyntaxError);
});
test('origin validation prevents unintended credential destinations', () => {
  assert.equal(normalizeBase(' https://linpro.top/ '), 'https://linpro.top');
  for (const value of ['http://linpro.top', 'https://user@linpro.top', 'https://linpro.top/api', 'https://linpro.top?x=1']) assert.throws(() => normalizeBase(value));
});
