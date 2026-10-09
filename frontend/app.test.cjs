const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const {randomUUID}=require('node:crypto');

test('text and image attachments preserve their payload when sending',()=>{
  const {context,state}=setup();
  state.attachments=[{name:'测试.txt',kind:'text',data:'你好🌏'},{name:'photo.png',kind:'image',data:'data:image/png;base64,AA=='}];
  const parts=context.buildUserContent('分析附件');
  assert.equal(parts[0].text,'分析附件');
  assert.match(parts[1].text,/测试.txt.*\n你好🌏/);
  assert.equal(parts[2].image_url.url,'data:image/png;base64,AA==');
});

test('stored image history restores structured content and escapes markup',()=>{
  const {context}=setup();
  const message=context.normalizeMessage({role:'user',content:JSON.stringify([{type:'text',text:'<script>alert(1)</script>'},{type:'image_url',image_url:{url:'data:image/png;base64,AA=='}}])});
  assert.ok(Array.isArray(message.content));
  const html=context.renderMessageContent(message.content);
  assert.ok(!html.includes('<script>'));
  assert.ok(html.includes('&lt;script&gt;'));
  assert.ok(html.includes('<img'));
  assert.equal(context.renderMessageContent([{type:'image_url',image_url:{url:'javascript:alert(1)'}}]),'');
});

test('unsupported and oversized attachments are rejected before reading',async()=>{
  const {context,state,alerts}=setup();
  await context.addFiles([{name:'large.txt',type:'text/plain',size:6*1024*1024},{name:'archive.zip',type:'application/zip',size:10}]);
  assert.equal(state.attachments.length,0);
  assert.equal(alerts.length,2);
});

test('attachment count limit prevents reading an additional file',async()=>{
  const {context,state,alerts}=setup();
  state.attachments=[{},{},{}];
  await context.addFiles([{name:'fourth.txt',type:'text/plain',size:1}]);
  assert.equal(state.attachments.length,3);
  assert.match(alerts[0],/3/);
});

function setup(){
  const elements=new Map();
  const element=selector=>{
    if(!elements.has(selector))elements.set(selector,{
      value:'',innerHTML:'',style:{},options:[],disabled:false,
      classList:{toggle(){},add(){},remove(){}},focus(){},querySelectorAll(){return []}
    });
    return elements.get(selector);
  };
  const calls=[],alerts=[];
  const context=vm.createContext({
    document:{querySelector:element},window:{addEventListener(){}},
    crypto:{randomUUID},localStorage:{getItem:()=> 'test-key'},
    requestAnimationFrame:callback=>callback(),console,
    confirm:()=>true,alert:message=>alerts.push(message),
    fetch:async(...args)=>{calls.push(args);return {ok:true,status:204}}
  });
  vm.runInContext(fs.readFileSync(__dirname+'/app.js','utf8'),context);
  const state=vm.runInContext('state',context);
  state.models=[{id:'model'}];
  state.conversations=[
    {id:'a',title:'A',persisted:true,messages:[{role:'user',content:'hello'}]},
    {id:'b',title:'B',persisted:true,messages:[]}
  ];
  state.activeId='a';state.messages=state.conversations[0].messages;
  return {context,state,calls,alerts,element};
}

test('cancel leaves history untouched and sends no request',async()=>{
  const {context,state,calls}=setup();context.confirm=()=>false;
  await context.deleteConversation('a');
  assert.equal(state.conversations.length,2);assert.equal(calls.length,0);
});

test('delete inactive history preserves the current messages',async()=>{
  const {context,state,calls}=setup();const messages=state.messages;
  await context.deleteConversation('b');
  assert.equal(state.activeId,'a');assert.equal(state.messages,messages);
  assert.equal(calls.length,1);assert.equal(calls[0][0],'/api/conversations?id=b');
  assert.equal(calls[0][1].method,'DELETE');assert.equal(calls[0][1].headers['X-API-Key'],'test-key');
  assert.equal(state.conversations.length,1);
});

test('delete active history clears messages, draft and attachments',async()=>{
  const {context,state,element}=setup();
  state.attachments=[{name:'file.txt',kind:'text',data:'hello'}];element('#prompt').value='draft';
  await context.deleteConversation('a');
  assert.notEqual(state.activeId,'a');assert.equal(state.messages.length,0);
  assert.equal(state.attachments.length,0);assert.equal(element('#prompt').value,'');
  assert.equal(state.conversations.some(c=>c.id==='a'),false);
  assert.equal(state.conversations[0].persisted,false);
});

test('unsaved empty chat can be removed without a server request',async()=>{
  const {context,state,calls}=setup();state.conversations[0].persisted=false;
  await context.deleteConversation('a');assert.equal(calls.length,0);
  assert.equal(state.conversations.some(c=>c.id==='a'),false);
});

for(const status of [401,500])test(`HTTP ${status} preserves history and restores controls`,async()=>{
  const {context,state,alerts,element}=setup();context.fetch=async()=>({ok:false,status});
  await context.deleteConversation('a');
  assert.equal(state.activeId,'a');assert.equal(state.conversations.length,2);
  assert.equal(state.messages[0].content,'hello');assert.equal(alerts.length,1);
  assert.equal(state.deletingId,null);assert.equal(element('#sendButton').disabled,false);
});

test('already deleted history is removed locally',async()=>{
  const {context,state}=setup();context.fetch=async()=>({ok:false,status:404});
  await context.deleteConversation('b');assert.equal(state.conversations.length,1);
});

test('streaming blocks deletion and keeps the composer draft',async()=>{
  const {context,state,calls,element}=setup();state.busy=true;element('#prompt').value='draft';
  await context.deleteConversation('a');element('#composer').onsubmit({preventDefault(){}});
  assert.equal(calls.length,0);assert.equal(state.conversations.length,2);
  assert.equal(element('#prompt').value,'draft');
});

test('pending deletion blocks duplicate requests and sending',async()=>{
  const {context,state,calls,element}=setup();let finish;
  context.fetch=(...args)=>{calls.push(args);return new Promise(resolve=>finish=resolve)};
  const pending=context.deleteConversation('a');
  assert.equal(element('#sendButton').disabled,true);
  await context.deleteConversation('a');await context.sendMessage('hello');
  assert.equal(calls.length,1);finish({ok:true,status:204});await pending;
  assert.equal(state.deletingId,null);
});

test('late message response cannot restore a deleted conversation',async()=>{
  const {context,state}=setup();let finish;
  context.fetch=async(url,options)=>options?.method==='DELETE'?{ok:true,status:204}:
    new Promise(resolve=>finish=()=>resolve({ok:true,json:async()=>({messages:[{role:'user',content:'stale'}]})}));
  const loading=context.selectConversation('a');await context.deleteConversation('a');
  finish();await loading;
  assert.equal(state.messages.length,0);assert.notEqual(state.activeId,'a');
});

test('deleting another conversation does not interrupt active history loading',async()=>{
  const {context,state}=setup();let finish;
  context.fetch=async(url,options)=>options?.method==='DELETE'?{ok:true,status:204}:
    new Promise(resolve=>finish=()=>resolve({ok:true,json:async()=>({messages:[{role:'user',content:'loaded'}]})}));
  const loading=context.selectConversation('a');await context.deleteConversation('b');
  finish();await loading;assert.equal(state.messages[0].content,'loaded');
});
