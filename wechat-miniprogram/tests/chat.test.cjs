const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createPage } = require('./page-harness.cjs');
const { normalizeBase } = require('../utils/api');
function setup() {
  const calls = [];
  const api = { normalizeBase,
    async request(base, key, path) {
      calls.push({ base, key, path });
      if (path === '/models') return { models: [{ id: 'one' }, { id: 'two' }], default: 'two' };
      if (path === '/conversations') return { conversations: [{ id: 'history/?中文', title: '旧对话', model: 'one' }] };
      return { messages: [{ role: 'user', content: '之前的问题' }, { role: 'assistant', content: '之前的回答' }] };
    },
    stream(base, key, body, delta, finish) { api.streamCall = { base, key, body, delta, finish }; return { abort() { finish(new Error('已停止')); } }; }
  };
  return { api, calls, page: createPage(api) };
}
test('connect, default model, send, stream, follow-up and model change', async () => {
  const { page, api } = setup(); page.data.key = ' key '; await page.connect();
  assert.equal(page.data.connected, true); assert.equal(page.data.modelIndex, 1); assert.equal(page.connection.key, 'key');
  page.data.input = '  你好  '; page.send();
  const id = page.conversationId;
  assert.equal(api.streamCall.body.model, 'two'); assert.equal(api.streamCall.body.messages[0].content, '你好');
  assert.equal(page.data.busy, true); api.streamCall.delta('你好🌏'); api.streamCall.finish();
  assert.equal(page.data.messages[1].content, '你好🌏'); assert.equal(page.data.busy, false);
  page.chooseModel({ detail: { value: '0' } }); page.data.input = '继续'; page.send();
  assert.equal(api.streamCall.body.messages.length, 3); assert.equal(api.streamCall.body.model, 'one');
  assert.equal(api.streamCall.body.conversation_id, id);
});
test('duplicate sends and navigation blocked while streaming; stop permits new chat', async () => {
  const { page, api, calls } = setup(); await page.connect(); page.data.input = '问题'; page.send();
  const id = page.conversationId; page.newChat(); page.send(); await page.history();
  assert.equal(page.conversationId, id); assert.equal(page.data.messages.length, 2); assert.equal(calls.length, 1);
  api.streamCall.delta('部分'); page.stop(); assert.equal(page.data.busy, false); assert.match(page.data.error, /停止/);
  assert.equal(page.data.messages[1].content, '部分'); page.newChat(); assert.equal(page.data.messages.length, 0);
});
test('history loads, escapes identifiers, restores model and continues context', async () => {
  const { page, calls, api } = setup(); await page.connect(); await page.history();
  page.data.input = '未发送'; await page.openConversation({ currentTarget: { dataset: { index: 0 } } });
  assert.equal(page.data.modelIndex, 0); assert.equal(page.data.input, ''); assert.equal(page.data.showHistory, false);
  assert.match(calls.at(-1).path, /history%2F%3F%E4%B8%AD%E6%96%87/);
  page.data.input = '继续'; page.send(); assert.equal(api.streamCall.body.messages.length, 3);
  assert.equal(api.streamCall.body.conversation_id, 'history/?中文');
});
test('failed connection and empty models keep send disabled', async () => {
  for (const result of [new Error('401 unauthorized'), { models: [], default: '' }]) {
    const { page, api } = setup(); api.request = async () => { if (result instanceof Error) throw result; return result; };
    await page.connect(); page.data.input = 'hi'; page.send();
    assert.equal(page.data.connected, false); assert.equal(page.data.loading, false); assert.ok(page.data.error); assert.equal(api.streamCall, undefined);
  }
});
test('reconnection resets account-specific conversation and history', async () => {
  const { page, api } = setup(); await page.connect(); page.data.input = 'secret'; page.send(); api.streamCall.finish();
  await page.history(); page.data.base = 'https://other.example'; page.data.key = 'other'; await page.connect();
  assert.equal(page.data.messages.length, 0); assert.equal(page.data.history.length, 0); assert.equal(page.connection.key, 'other');
});
test('opening missing history reports an error outside the overlay', async () => {
  const { page, api } = setup(); await page.connect(); await page.history();
  api.request = async () => { throw new Error('404 missing'); };
  await page.openConversation({ currentTarget: { dataset: { index: 0 } } });
  assert.match(page.data.error, /404/); assert.equal(page.data.showHistory, false);
});
test('unloaded page ignores pending connection result', async () => {
  const { page, api } = setup(); let resolve;
  api.request = () => new Promise(r => resolve = r);
  const pending = page.connect(); page.onUnload(); const updates = page.updates;
  resolve({ models: [{ id: 'one' }], default: 'one' }); await pending;
  assert.equal(page.updates, updates);
});
test('failure preserves partial reply and next send excludes empty assistant', async () => {
  const { page, api } = setup(); await page.connect(); page.data.input = 'first'; page.send(); api.streamCall.finish(new Error('network'));
  assert.equal(page.data.busy, false); page.data.input = 'retry'; page.send();
  assert.equal(api.streamCall.body.messages.filter(m => m.role === 'assistant').length, 0);
});
