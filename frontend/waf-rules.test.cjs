const {test}=require('node:test');
const assert=require('node:assert/strict');
const core=require('./waf-rules-core.js');
const fixture=()=>({notes:['草案'],rules:[{waf_id:null,name:'SQL 检测',cve:'-',type:'SQL',payload:'GET /?q=<script> HTTP/1.1',ackey:["a'b",'第二行'],rule:'检测说明',regex:'\\bif\\s*\\(',zone:['ARGS'],phase:[],risk:'3',score:{value:'12',strict:null,normal:null,category:'$SQL'},needs_chain:false,needs_review:['确认配置']}]});
test('form round trip preserves regex, multiline ackey, Unicode and score string',()=>{
 const data=fixture();data.rules[0].phase=['request'];
 const request=core.requests(core.validate(data),'https://waf.example.com')[0];
 const form=new URLSearchParams(request.body);
 assert.equal(form.get('pattern'),data.rules[0].regex);
 assert.equal(form.get('ackey'),"a'b\n第二行");
 assert.deepEqual(JSON.parse(form.get('score_json')),{'$SQL':'12'});
 assert.equal(form.get('mode'),'true');assert.equal(form.has('waf_id'),false);
 assert.match(request.curl,/--data-raw/);assert.equal(request.status,'draft');
});
test('missing phase and chain conditions cannot produce executable curls',()=>{
 const data=fixture();assert.equal(core.requests(data,'https://example.com')[0].curl,null);
 data.rules[0].needs_chain=true;const result=core.requests(data,'https://example.com')[0];
 assert.equal(result.form,null);assert.equal(result.body,null);assert.equal(result.status,'requires_chain');
});
test('untrusted payload and model output are escaped in table and exported document',()=>{
 const data=fixture();data.notes=['<img src=x onerror=alert(1)>'];
 assert.ok(core.documentHTML(data).includes('&lt;script&gt;'));
 assert.ok(!core.documentHTML(data).includes('<img'));
 assert.ok(core.table(data).includes('12/待确认/待确认'));
});
test('invalid results and unsafe base URLs are rejected',()=>{
 for(const value of [{}, {notes:[],rules:[{}]}])assert.throws(()=>core.validate(value));
 const data=fixture();data.rules[0].waf_id=19197;assert.throws(()=>core.validate(data));
 for(const url of ['javascript:alert(1)','https://user:pass@example.com','https://example.com/path','https://example.com/?q=x']) assert.throws(()=>core.requests(fixture(),url));
});
function response(parts){return new Response(new ReadableStream({start(c){for(const p of parts)c.enqueue(new TextEncoder().encode(p));c.close();}}));}
test('SSE parses chunk boundaries, CRLF, Unicode and DONE',async()=>{
 const text=await core.readStream(response(['data: {"del','ta":"规则"}\r','\n\r\ndata: {"delta":"完成"}\n\ndata: [DONE]\n\n']),()=>{});
 assert.equal(text,'规则完成');
});
test('SSE rejects provider errors and truncated streams',async()=>{
 await assert.rejects(core.readStream(response(['data: {"error":"provider failed"}\n\n']),()=>{}),/provider failed/);
 await assert.rejects(core.readStream(response(['data: {"delta":"partial"}\n\n']),()=>{}),/提前结束/);
});
