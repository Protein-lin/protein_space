const api = require('../../utils/api');
Page({
  data: { base: 'https://www.linpro.top', key: '', settings: true, connected: false, loading: false, busy: false, models: [], modelIndex: 0, messages: [], input: '', error: '', history: [], showHistory: false, anchor: '' },
  onLoad() { const saved = wx.getStorageSync('protein-base'); this.setData({ base: !saved || saved === 'https://linpro.top' ? this.data.base : saved }); this.newChat(); },
  onUnload() { this.disposed = true; if (this.active) this.active.abort(); },
  field(e) { this.setData({ [e.currentTarget.dataset.field]: e.detail.value }); },
  toggleSettings() { if (!this.data.busy && !this.data.loading) this.setData({ settings: !this.data.settings }); },
  async connect() {
    if (this.data.busy || this.data.loading) return;
    this.setData({ loading: true, error: '', connected: false, models: [], history: [] });
    try {
      const base = api.normalizeBase(this.data.base), key = this.data.key.trim();
      const result = await api.request(base, key, '/models');
      if (this.disposed) return;
      const models = result.models || [];
      if (!models.length) throw new Error('没有可用模型，请检查账号模型权限');
      this.connection = { base, key }; wx.setStorageSync('protein-base', base);
      this.setData({ base, key, models, modelIndex: Math.max(0, models.findIndex(m => m.id === result.default)), connected: true, settings: false });
      this.newChat();
    } catch (e) { if (!this.disposed) this.setData({ error: e.message }); }
    finally { if (!this.disposed) this.setData({ loading: false }); }
  },
  newChat() {
    if (this.data.busy) return;
    this.conversationId = 'wx-' + Date.now() + '-' + Math.random().toString(36).slice(2);
    this.setData({ messages: [], input: '', error: '', showHistory: false });
  },
  chooseModel(e) { this.setData({ modelIndex: Number(e.detail.value) }); },
  async history() {
    if (!this.data.connected || this.data.busy || this.data.loading) return;
    this.setData({ loading: true, error: '' });
    try {
      const c = this.connection, result = await api.request(c.base, c.key, '/conversations');
      if (this.disposed) return;
      this.setData({ history: result.conversations || [], showHistory: true });
    } catch (e) { if (!this.disposed) this.setData({ error: e.message }); }
    finally { if (!this.disposed) this.setData({ loading: false }); }
  },
  closeHistory() { this.setData({ showHistory: false }); },
  async openConversation(e) {
    if (this.data.loading || this.data.busy) return;
    const conversation = this.data.history[e.currentTarget.dataset.index];
    this.setData({ loading: true, error: '' });
    try {
      const c = this.connection, result = await api.request(c.base, c.key, '/conversations/messages?id=' + encodeURIComponent(conversation.id));
      if (this.disposed) return;
      this.conversationId = conversation.id;
      this.setData({ messages: result.messages || [], input: '', showHistory: false, modelIndex: Math.max(0, this.data.models.findIndex(m => m.id === conversation.model)) });
      this.scroll();
    } catch (e) { if (!this.disposed) this.setData({ error: e.message, showHistory: false }); }
    finally { if (!this.disposed) this.setData({ loading: false }); }
  },
  scroll() { this.setData({ anchor: '' }, () => this.setData({ anchor: 'bottom' })); },
  send() {
    const text = this.data.input.trim();
    if (!text || !this.data.connected || this.data.busy || this.data.loading) return;
    const messages = this.data.messages.filter(m => m.content).concat({ role: 'user', content: text });
    const body = { conversation_id: this.conversationId, model: this.data.models[this.data.modelIndex].id, messages };
    this.setData({ messages: messages.concat({ role: 'assistant', content: '' }), input: '', busy: true, error: '' }); this.scroll();
    const index = messages.length, c = this.connection;
    this.active = api.stream(c.base, c.key, body, delta => {
      if (this.disposed) return;
      this.setData({ [`messages[${index}].content`]: this.data.messages[index].content + delta }); this.scroll();
    }, error => {
      if (this.disposed) return;
      this.active = null; this.setData({ busy: false, error: error ? error.message : '' });
    });
  },
  stop() { if (this.active) this.active.abort(); }
});
