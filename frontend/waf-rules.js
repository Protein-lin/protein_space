(() => {
  'use strict';
  const $ = id => document.getElementById(id), core=window.WafRules;
  let data=null, controller=null, modelCount=0, catalog=[], original=null, reviewID=null;
  function status(text,error=false) { $('status').textContent=text; $('status').classList.toggle('error',error); }
  function busy(value) {
    $('generate').disabled=value||!modelCount; $('cancel').disabled=!value;
    ['saveReview','listReviews','provider','model','evidence','deploymentContext','sourceFile','upload','baseURL','applyJSON','jsonEditor','exportHTML','exportJSON'].forEach(id=>$(id).disabled=value);
    $('model').disabled=value||!modelCount;
  }
  function render(next) {
    const items=core.requests(next,$('baseURL').value.trim());
    data=next;
    $('ruleTable').innerHTML=core.table(data);
    editTable();
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
  function normalize(next) {
    next.rules.forEach(r=>{const c=catalog.find(c=>c.name===r.score.category);if(!c)throw Error('未知计分类别：'+r.score.category);r.type=c.name.slice(1);r.attack_name=c.attack_name;r.score.strict=c.strict_score;r.score.normal=c.general_score;});
    return core.validate(next);
  }
  function editTable() {
    const keys=['','name','cve','type','payload','ackey','rule','regex','zone','score'];
    $('ruleTable').querySelectorAll('tbody tr').forEach((row,index)=>{
      const r=data.rules[index];
      [...row.cells].forEach((cell,col)=>{
        if(!col)return;
        const key=keys[col], input=document.createElement(key==='type'?'select':'textarea');
        input.setAttribute('aria-label',`${index+1} ${key}`);
        if(key==='type') {catalog.forEach(c=>input.add(new Option(c.name+' · '+c.attack_name,c.name)));input.value=r.score.category;}
        else input.value=key==='score'?r.score.value:Array.isArray(r[key])?r[key].join('\n'):r[key];
        input.onchange=()=>{
          const next=JSON.parse(JSON.stringify(data)),target=next.rules[index];
          if(key==='type')target.score.category=input.value;
          else if(key==='score')target.score.value=input.value;
          else target[key]=['ackey','zone'].includes(key)?input.value.split(/\n/).filter(Boolean):input.value;
          try{render(normalize(next));$('jsonEditor').value=JSON.stringify(data,null,2);status('复核修改已应用，保存后用于后续生成');}catch(e){status(e.message,true);}
        };
        cell.replaceChildren(input);
        if(key==='score'){const hint=document.createElement('small');hint.textContent=`严格 ${r.score.strict} / 普通 ${r.score.normal}`;cell.append(hint);}
      });
    });
  }
  $('saveReview').onclick=async()=>{
    if(!data)return;
    try{
      const response=await fetch('/api/waf-rules/reviews',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:reviewID||(reviewID=crypto.randomUUID()),example:{original,corrected:data}})});
      if(!response.ok)throw Error(await response.text());status('已保存到当前账号；后续生成将参考最近的修订经验');
    }catch(e){status(e.message,true);}
  };
  $('listReviews').onclick=async()=>{
    try{const response=await fetch('/api/waf-rules/reviews');if(!response.ok)throw Error(await response.text());const list=await response.json();$('reviewList').replaceChildren();
      for(const item of list){const line=document.createElement('div'),button=document.createElement('button');line.textContent=item.example.corrected?.rules?.map(r=>r.name).join('、')||item.id;button.textContent='删除此经验';button.onclick=async()=>{try{const res=await fetch('/api/waf-rules/reviews?id='+encodeURIComponent(item.id),{method:'DELETE'});if(!res.ok)throw Error(await res.text());line.remove();}catch(e){status(e.message,true);}};line.append(button);$('reviewList').append(line);}
      if(!list.length)$('reviewList').textContent='暂无修订经验';
    }catch(e){status(e.message,true);}
  };
  async function models() {
    try {
      const cat=await fetch('/api/waf-rules/catalog');if(!cat.ok)throw Error('类型配置加载失败');catalog=await cat.json();
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
    controller=new AbortController();busy(true);status('Agent 正在识别输入并分析 PoC，随后生成规则表格（两次模型请求）…');
    try {
      ModelPreference.remember($('model').value);
      const response=await fetch('/api/waf-rules/generate',{method:'POST',headers:{'Content-Type':'application/json'},signal:controller.signal,body:JSON.stringify({input:$('evidence').value,context:$('deploymentContext').value,model:$('model').value,provider_config_id:Number($('provider').value)})});
      const text=await core.readStream(response,value=>{$('jsonEditor').value=value;});
      const result=normalize(core.parse(text));original=JSON.parse(JSON.stringify(result));reviewID=null;render(result);$('jsonEditor').value=JSON.stringify(result,null,2);
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
  $('applyJSON').onclick=()=>{try{render(normalize(core.parse($('jsonEditor').value)));status('修改已应用，请复核规则和接口数据')}catch(error){status(error.message,true)}};
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
