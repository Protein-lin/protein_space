// Buffer bytes until an SSE line ends: UTF-8 characters may span network chunks.
function decode(bytes) {
  return decodeURIComponent(Array.from(bytes, b => '%' + b.toString(16).padStart(2, '0')).join(''));
}
class SSEParser {
  constructor(onDelta) { this.onDelta = onDelta; this.bytes = []; this.data = []; this.done = false; }
  push(buffer) {
    for (const b of new Uint8Array(buffer)) {
      if (b === 10) {
        const line = decode(this.bytes).replace(/\r$/, ''); this.bytes = [];
        this.line(line);
      } else this.bytes.push(b);
    }
  }
  line(line) {
    if (this.done) return;
    if (line === '') { this.event(); return; }
    if (line.startsWith('data:')) this.data.push(line.slice(5).replace(/^ /, ''));
  }
  event() {
    if (!this.data.length || this.done) return;
    const payload = this.data.join('\n'); this.data = [];
    if (payload.trim() === '[DONE]') { this.done = true; return; }
    const event = JSON.parse(payload);
    if (event.error) throw new Error(typeof event.error === 'string' ? event.error : JSON.stringify(event.error));
    const delta = event.delta || event.content || '';
    if (typeof delta !== 'string') throw new Error('服务器返回了不支持的消息格式');
    this.onDelta(delta);
  }
  finish() {
    if (this.bytes.length) { this.line(decode(this.bytes).replace(/\r$/, '')); this.bytes = []; }
    this.event();
    if (!this.done) throw new Error('连接提前结束，回复可能不完整');
  }
}
module.exports = { SSEParser };
