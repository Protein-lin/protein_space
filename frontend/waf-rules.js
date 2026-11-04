(() => {
  'use strict';
  const $ = id => document.getElementById(id), core=window.WafRules;
  let data=null, controller=null, modelCount=0;
  function status(text,error=false) { $('status').textContent=text; $('status').classList.toggle('error',error); }
  function busy(value) {
    $('generate').disabled=value||!modelCount; $('cancel').disabled=!value;
    ['provider','model','evidence','deploymentContext','sourceFile','upload','baseURL','applyJSON','jsonEditor','exportHTML','exportJSON'].forEach(id=>$(id).disabled=value);
    $('model').disabled=value||!modelCount;
  }
  function render(next) {
    const items=core.requests(next,$('baseURL').value.trim());
    data=next;
    $('ruleTable').innerHTML=core.table(data);
    $('notes').replaceChildren();
    const list=document.createElement('ul');
    [...data.notes,...data.rules.flatMap((r,i)=>r.needs_review.map(n=>`${i+1}. ${n}`))].forEach(note=>{const li=document.createElement('li');li.textContent=note;list.append(li)});
    $('notes').append(list);
    $('requests').replaceChildren();
    items.forEach(item=>{
      const details=document.createElement('details'), title=document.createElement('summary'), pre=document.createElement('pre');
      title.textContent=`${item.index}. ${item.name} · ${item.status==='requires_chain'?'需要链式规则适配':item.missing.length?'待补全 '+item.missing.join(', '):'接口草案'}`;
      pre.textContent=JSON.stringify(item,null,2); details.append(title,pre);
      if(item.curl) {
        const curl=document.createElement('pre'),button=document.createElement('button'); curl.textContent=item.curl;button.textContent='复制 cURL';button.type='button';
        button.onclick=async()=>{try{await navigator.clipboard.writeText(item.curl);status('已复制 cURL；cookies.txt 需使用目标系统的有效登录会话')}catch{status('复制失败，请从请求区域手动复制',true)}};
        details.append(curl,button);
      }
      $('requests').append(details);
    });
    $('output').classList.remove('hidden');
  }
  async function models() {
    try {
      const response=await fetch('/api/models');
      if(!response.ok) throw Error(response.status===401?'请先登录以加载模型':'模型服务不可用');
      const payload=await response.json();
      const options=(payload.models||[]).filter(m=>typeof m.id==='string'&&m.id);
      $('model').replaceChildren(...options.map(m=>new Option(m.name||m.id,m.id)));
      modelCount=options.length;
      $('model').value=ModelPreference.choose(options,payload.default);
      if(!controller){busy(false);status(modelCount?'模型已连接，可以开始生成':'暂无可用模型，请配置 Agent 服务');}
    } catch(error) { modelCount=0;$('model').replaceChildren(new Option('模型不可用',''));if(!controller){busy(false);status(error.message,true);} }
  }
  $('generateForm').onsubmit=async event=>{
    event.preventDefault();if(controller)return;
    try{core.requests({rules:[]},$('baseURL').value.trim());}catch(error){status(error.message,true);return;}
    if(new TextEncoder().encode($('evidence').value).length>180*1024){status('输入超过 180 KB，请减少日志内容',true);return;}
    data=null;$('output').classList.add('hidden');$('jsonEditor').value='';
    controller=new AbortController();busy(true);status('Agent 正在分析并生成规则…');
    try {
      ModelPreference.remember($('model').value);
      const response=await fetch('/api/waf-rules/generate',{method:'POST',headers:{'Content-Type':'application/json'},signal:controller.signal,body:JSON.stringify({input:$('evidence').value,context:$('deploymentContext').value,model:$('model').value,provider_config_id:Number($('provider').value)})});
      const text=await core.readStream(response,value=>{$('jsonEditor').value=value;});
      const result=core.parse(text);render(result);$('jsonEditor').value=JSON.stringify(result,null,2);
      status(result.rules.length?`已生成 ${result.rules.length} 条规则草案，请检查分析说明和待补全字段`:'证据不足，未生成规则；请查看分析说明');
    } catch(error) { status(error.name==='AbortError'?'已停止生成；未完成的输出保留在 JSON 编辑器中':`生成失败：${error.message}`,true); }
    finally {controller=null;busy(false);}
  };
  $('cancel').onclick=()=>controller?.abort();
  $('upload').onclick=()=>$('sourceFile').click();
  $('sourceFile').onchange=async event=>{
    const file=event.target.files[0];if(!file)return;
    try {
      if(file.size>180*1024)throw Error('文件不能超过 180 KB');
      if(!/\.(txt|log|json|md|ya?ml|py|http|xml|csv)$/i.test(file.name))throw Error('请选择支持的文本文件');
      const text=await file.text();if(text.includes('\0'))throw Error('文件不是有效文本');
      $('evidence').value=text;status(`已导入 ${file.name}`);
    }catch(error){status(error.message,true);}finally{event.target.value='';}
  };
  $('applyJSON').onclick=()=>{try{render(core.parse($('jsonEditor').value));status('修改已应用，请复核规则和接口数据')}catch(error){status(error.message,true)}};
  $('baseURL').onchange=()=>{if(data)try{render(data)}catch(error){status(error.message,true)}};
  function download(name,type,text) {
    const url=URL.createObjectURL(new Blob([text],{type})),link=document.createElement('a');link.href=url;link.download=name;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
  }
  $('exportHTML').onclick=()=>{if(data)download('waf-rules.html','text/html;charset=utf-8',core.documentHTML(data));};
  $('exportJSON').onclick=()=>{if(data)try{download('waf-rule-requests.json','application/json;charset=utf-8',JSON.stringify({document:data,requests:core.requests(data,$('baseURL').value.trim())},null,2));}catch(error){status(error.message,true)}};
  async function providers() {
    try {
      const response=await fetch('/api/provider-configs');
      if(!response.ok)return;
      const payload=await response.json();
      $('provider').replaceChildren(new Option('默认 Agent 服务','0'),...(payload.configs||[]).map(p=>new Option(p.name,String(p.id))));
    }catch { /* Default Agent remains available for guests. */ }
  }
  window.addEventListener('auth:ready',()=>{models();providers();});
})();
