const state={conversations:[],activeId:null,messages:[],busy:false,models:[],defaultModel:'',attachments:[],deletingId:null,conversationLoad:0};
const $=s=>document.querySelector(s);const authHeaders=()=>{const key=localStorage.getItem('waf_api_key');return key?{'X-API-Key':key}:{}};
function escapeHTML(s){return s.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function icon(name){const paths={image:'<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-4.5-4.5L7 20"/>',file:'<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"/><path d="M14 2v6h6M8 13h8M8 17h6"/>',close:'<path d="m7 7 10 10M17 7 7 17"/>'};return `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name]||''}</svg>`}
function contentText(content){if(Array.isArray(content))return content.filter(x=>x&&x.type==='text').map(x=>x.text||'').join('\n');if(typeof content==='string'){try{const parsed=JSON.parse(content);if(Array.isArray(parsed))return contentText(parsed)}catch(e){}return content}return ''}
function normalizeMessage(message){if(typeof message.content==='string'){try{const parsed=JSON.parse(message.content);if(Array.isArray(parsed))return {...message,content:parsed}}catch(e){}}return message}
function renderMessageContent(content){if(!Array.isArray(content))return escapeHTML(contentText(content));return content.map(part=>{if(part.type==='image_url'&&part.image_url?.url?.startsWith('data:image/'))return `<img class="message-image" src="${escapeHTML(part.image_url.url)}" alt="上传的图片">`;if(part.type==='text')return escapeHTML(part.text||'');return ''}).join('')}
function syncModel(model){const select=$('#modelSelect');if(!select||!model)return;if([...select.options].some(x=>x.value===model))select.value=model}
function scrollMessagesToBottom(){requestAnimationFrame(()=>{const box=$('#messages');if(box)box.scrollTop=box.scrollHeight})}
function renderConversations(){
  $('#newChat').disabled=state.busy||!!state.deletingId;
  const el=$('#conversationList');
  el.innerHTML=state.conversations.map(c=>`<div class="conversation ${c.id===state.activeId?'active':''}" data-id="${escapeHTML(c.id)}"><span>${escapeHTML(c.title||'新对话')}</span><button type="button" class="delete-conversation" title="删除对话" aria-label="删除对话：${escapeHTML(c.title||'新对话')}" ${state.busy||state.deletingId?'disabled':''}>${state.deletingId===c.id?'删除中…':'删除'}</button></div>`).join('');
  el.querySelectorAll('.conversation').forEach(row=>{
    row.onclick=()=>selectConversation(row.dataset.id);
    row.querySelector('.delete-conversation').onclick=event=>{event.stopPropagation();deleteConversation(row.dataset.id)};
  });
}
async function selectConversation(id){
  const item=state.conversations.find(c=>c.id===id);
  if(!item||state.busy||state.deletingId)return;
  const request=++state.conversationLoad;
  state.activeId=id;
  state.messages=item.messages||[];
  syncModel(item.model);
  renderConversations();renderMessages();
  if(!item.persisted)return;
  try{
    const response=await fetch('/api/conversations/messages?id='+encodeURIComponent(id),{headers:authHeaders()});
    if(!response.ok)throw new Error(await response.text());
    const data=await response.json();
    if(request!==state.conversationLoad||state.activeId!==id||!state.conversations.includes(item)||state.busy)return;
    state.messages=(data.messages||[]).map(normalizeMessage);
    item.messages=state.messages;
    renderMessages();
  }catch(error){console.error('load conversation failed',error)}
}
async function deleteConversation(id){
  const item=state.conversations.find(c=>c.id===id);
  if(!item||state.busy||state.deletingId)return;
  if(!confirm(`确定删除“${item.title||'新对话'}”吗？该对话的所有消息将被永久删除，无法恢复。`))return;
  state.deletingId=id;
  renderConversations();
  $('#sendButton').disabled=true;
  try{
    if(item.persisted){
      const response=await fetch('/api/conversations?id='+encodeURIComponent(id),{method:'DELETE',headers:authHeaders()});
      // A record removed in another tab is already in the desired state.
      if(!response.ok&&response.status!==404)throw new Error(response.status===401?'登录已过期，请重新登录后重试':'删除失败，请稍后重试');
    }
    state.conversations=state.conversations.filter(c=>c.id!==id);
    if(state.activeId===id){
      state.attachments=[];renderAttachments();
      $('#prompt').value='';$('#prompt').style.height='auto';
      newChat();
    }
  }catch(error){alert(error.message||'删除失败，请稍后重试')}
  finally{
    state.deletingId=null;
    renderConversations();
    $('#sendButton').disabled=state.busy||state.models.length===0;
  }
}
function renderMessages(){const box=$('#messages');box.innerHTML=state.messages.map(m=>`<div class="message ${m.role}"><div class="bubble">${renderMessageContent(m.content)}</div></div>`).join('');scrollMessagesToBottom();$('#welcome').classList.toggle('hidden',state.messages.length>0)}
function newChat(){if(state.busy)return;++state.conversationLoad;state.activeId=crypto.randomUUID();state.messages=[];state.conversations.unshift({id:state.activeId,title:'新对话',persisted:false,model:$('#modelSelect').value||state.defaultModel,messages:state.messages});renderConversations();renderMessages();$('#prompt').focus()}
function addMessage(role,content){const m={role,content};state.messages.push(m);const c=state.conversations.find(c=>c.id===state.activeId);if(c){c.messages=state.messages;if(role==='user'&&c.title==='新对话')c.title=contentText(content).slice(0,24)||'附件对话'}renderConversations();renderMessages();return m}
async function loadModels(){try{const r=await fetch('/api/models');if(!r.ok)throw new Error(await r.text());const d=await r.json();state.models=d.models||[];state.defaultModel=d.default||state.models[0]?.id||'';const select=$('#modelSelect');select.innerHTML=state.models.map(m=>`<option value="${escapeHTML(m.id)}">${escapeHTML(m.name||m.id)}</option>`).join('');syncModel(state.conversations.find(c=>c.id===state.activeId)?.model||state.defaultModel);select.disabled=state.models.length===0;$('#statusDot').className='ok';$('#statusText').textContent=state.models.length?'服务已连接':'暂无可用模型'}catch(e){$('#modelSelect').innerHTML='<option value="">请先登录</option>';$('#modelSelect').disabled=true;$('#statusDot').className='bad';$('#statusText').textContent='模型服务未连接'}}
async function loadConversations(){
  const request=++state.conversationLoad;
  const conversations=state.conversations;
  try{
    const response=await fetch('/api/conversations',{headers:authHeaders()});
    if(!response.ok)return;
    const data=await response.json();
    if(request!==state.conversationLoad||state.conversations!==conversations||state.busy||state.deletingId)return;
    state.conversations=(data.conversations||[]).map(c=>({...c,messages:[],persisted:true}));
    if(state.conversations.length)await selectConversation(state.conversations[0].id);
    else newChat();
  }catch(error){console.debug('history unavailable',error)}
}
function renderAttachments(){const el=$('#attachmentList');el.innerHTML=state.attachments.map((file,index)=>`<div class="attachment-item">${file.kind==='image'?`<img src="${escapeHTML(file.data)}" alt="">`:`<span class="attachment-icon">${icon('file')}</span>`}<span class="attachment-name">${escapeHTML(file.name)}</span><button type="button" data-remove-attachment="${index}" aria-label="移除 ${escapeHTML(file.name)}">${icon('close')}</button></div>`).join('');el.classList.toggle('hidden',state.attachments.length===0)}
function readFile(file){return new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(reader.result);reader.onerror=()=>reject(new Error(`读取文件失败：${file.name}`));if(file.type.startsWith('image/'))reader.readAsDataURL(file);else reader.readAsText(file)})}
async function addFiles(files){for(const file of files){if(state.attachments.length>=3){alert('最多同时上传 3 个文件');break}if(file.size>5*1024*1024){alert(`${file.name} 超过 5MB 限制`);continue}const isImage=file.type.startsWith('image/');const isText=!isImage&&(/\.(txt|md|csv|json|log|xml|yaml|yml)$/i.test(file.name)||file.type.startsWith('text/'));if(!isImage&&!isText){alert(`${file.name} 暂不支持，请上传图片或文本文件`);continue}try{const data=await readFile(file);state.attachments.push({name:file.name,kind:isImage?'image':'text',data});renderAttachments()}catch(e){alert(e.message)}}}
function buildUserContent(text){const parts=[];if(text)parts.push({type:'text',text});for(const file of state.attachments){if(file.kind==='image')parts.push({type:'image_url',image_url:{url:file.data}});else parts.push({type:'text',text:`\n\n[文件：${file.name}]\n${file.data}`})}return parts.length===1&&parts[0].type==='text'?parts[0].text:parts}
async function sendMessage(text){if(state.busy||state.deletingId)return;if(!$('#modelSelect').value){alert('请先选择一个模型');return}if(!text&&state.attachments.length===0)return;if(!state.activeId)newChat();const model=$('#modelSelect').value;const conversation=state.conversations.find(c=>c.id===state.activeId);if(conversation){conversation.model=model;conversation.persisted=true;}const userContent=buildUserContent(text);state.attachments=[];renderAttachments();addMessage('user',userContent);const assistant=addMessage('assistant','');const assistantBubble=$('#messages .assistant:last-child .bubble');let queue='',streamDone=false,finishTyping;const typingDone=new Promise(resolve=>finishTyping=resolve);const paint=loading=>{if(!assistantBubble)return;assistantBubble.textContent=assistant.content;if(loading){const dots=document.createElement('span');dots.className='typing-dots';dots.innerHTML='<i></i><i></i><i></i>';assistantBubble.appendChild(dots)}scrollMessagesToBottom()};const typingTimer=setInterval(()=>{if(queue){assistant.content+=queue[0];queue=queue.slice(1);paint(false)}else if(streamDone){clearInterval(typingTimer);paint(false);finishTyping()}else{paint(true)}},24);const push=chunk=>{queue+=chunk};state.busy=true;++state.conversationLoad;renderConversations();$('#sendButton').disabled=true;$('#modelSelect').disabled=true;try{const r=await fetch('/api/chat/stream',{method:'POST',headers:{'Content-Type':'application/json',...authHeaders()},body:JSON.stringify({conversation_id:state.activeId,model,messages:state.messages.filter(x=>x!==assistant)})});if(!r.ok||!r.body){const detail=await r.text();if(r.status===413)throw new Error('图片或文件过大，请压缩后重试');throw new Error(detail||`请求失败（HTTP ${r.status}）`)}const reader=r.body.getReader(),decoder=new TextDecoder();let buffer='';while(true){const {value,done}=await reader.read();if(done)break;buffer+=decoder.decode(value,{stream:true});const parts=buffer.split('\n\n');buffer=parts.pop();for(const part of parts){const line=part.split('\n').find(x=>x.startsWith('data:'));if(!line)continue;const payload=line.slice(5).trim();if(payload==='[DONE]')continue;try{const evt=JSON.parse(payload);if(evt.error)throw new Error(evt.error);push(evt.delta||evt.content||'')}catch(e){if(e.message)throw e}}}}catch(e){push(`\n\n请求失败：${e.message}`)}finally{streamDone=true;await typingDone;state.busy=false;renderConversations();$('#sendButton').disabled=state.models.length===0;$('#modelSelect').disabled=state.models.length===0}}
function showChat(){ $('#homePage').classList.add('hidden'); $('#chatShell').classList.remove('hidden'); if(!state.activeId)newChat(); scrollMessagesToBottom(); }
$('#enterChat').onclick=showChat;$('#backHome').onclick=()=>{ $('#chatShell').classList.add('hidden'); $('#homePage').classList.remove('hidden'); };$('#newChat').onclick=newChat;$('#attachButton').onclick=()=>$('#fileInput').click();$('#fileInput').onchange=e=>{addFiles([...e.target.files]);e.target.value=''};$('#attachmentList').onclick=e=>{const index=e.target.dataset.removeAttachment;if(index!==undefined){state.attachments.splice(Number(index),1);renderAttachments()}};$('#prompt').onpaste=e=>{const files=[...e.clipboardData.files];if(files.length){e.preventDefault();addFiles(files)}};$('#modelSelect').onchange=()=>{const c=state.conversations.find(c=>c.id===state.activeId);if(c)c.model=$('#modelSelect').value};$('#composer').onsubmit=e=>{e.preventDefault();if(state.busy||state.deletingId)return;const input=$('#prompt');const text=input.value.trim();if(text||state.attachments.length){input.value='';input.style.height='auto';sendMessage(text)}};$('#prompt').onkeydown=e=>{if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();$('#composer').requestSubmit()}};newChat();window.addEventListener('auth:ready',()=>{loadModels();loadConversations()});
