(function(root) {
  'use strict';
  const columns = ['waf_id','name','cve','type','payload','ackey','rule','regex','zone','score(设置分数/严格/普通阈值)'];
  const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  function validate(data) {
    const strings = v => Array.isArray(v) && v.every(x => typeof x === 'string');
    if (!data || !strings(data.notes) || !Array.isArray(data.rules) || data.rules.length > 30) throw Error('结果必须包含 notes 和 rules 数组（最多 30 条）');
    data.rules.forEach((r,i) => {
      const fail = () => { throw Error(`第 ${i+1} 条规则字段格式无效，请检查 JSON`); };
      if (!r || !['name','cve','type','payload','rule','regex','risk'].every(k => typeof r[k] === 'string')) fail();
      if (!['ackey','zone','phase','needs_review'].every(k => strings(r[k]))) fail();
      if (!r.ackey.length || r.ackey.some(x=>!x.trim()) || !r.name.trim() || !r.regex.trim() || !/^\d{1,2}$/.test(r.risk)) fail();
      if (r.waf_id !== null || typeof r.needs_chain !== 'boolean') fail();
      const s = r.score;
      if (!s || typeof s.value !== 'string' || !/^\d{1,3}$/.test(s.value) || typeof s.category !== 'string' || !/^\$[A-Za-z0-9_]+$/.test(s.category)) fail();
      if (![s.strict,s.normal].every(n => n === null || (Number.isSafeInteger(n) && n >= 0))) fail();
      if ([...r.phase,...r.zone].some(v=>!v.trim() || /[,\r\n]/.test(v))) fail();
    });
    return data;
  }
  function parse(text) { return validate(JSON.parse(text.trim().replace(/^```(?:json)?\s*/i,'').replace(/\s*```$/,''))); }
  function cells(r) { return ['待分配',r.name,r.cve,r.type,r.payload,r.ackey.join('\n'),r.rule,r.regex,r.zone.join(', '),`${r.score.value}/${r.score.strict ?? '待确认'}/${r.score.normal ?? '待确认'}`]; }
  function table(data) {
    return `<table><thead><tr>${columns.map(c=>`<th>${escape(c)}</th>`).join('')}</tr></thead><tbody>${data.rules.map(r=>`<tr>${cells(r).map(v=>`<td>${escape(v)}</td>`).join('')}</tr>`).join('')}</tbody></table>`;
  }
  function form(r) {
    if (r.needs_chain) return null;
    return {ackey:r.ackey.join('\n'),operator:'rx',pattern:r.regex,phase:r.phase.join(','),zone:r.zone.join(','),risk:r.risk,score_json:JSON.stringify({[r.score.category]:r.score.value}),mode:'true',fac:'false',base64_decode:'false',usetype:'default',configuration:'',message:'',headers_key:'',set_headers_key:'',body_key:'',set_body_key:'',option_key:'',set_option_key:'',textarea_key:'',set_textarea_key:''};
  }
  const shell = value => "'" + String(value).replace(/'/g, "'\"'\"'") + "'";
  function requests(data, base) {
    const url = new URL(base);
    if (!['http:','https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || (url.pathname !== '/' && url.pathname !== '')) throw Error('WAF 地址应为 http(s)://主机[:端口]，不含路径或认证信息');
    return data.rules.map((r,i) => {
      const fields=form(r), missing=[];
      if(!r.phase.length) missing.push('phase');
      if(!r.zone.length) missing.push('zone');
      return {index:i+1, name:r.name, status:r.needs_chain?'requires_chain':missing.length?'incomplete':'draft', needs_review:[...r.needs_review,'需在目标 WAF 数据库确认 phase/zone 关系、operator、计分类别；登录用户须具有 rule_edit 权限'], missing, method:'POST',url:url.origin+'/waf_rule/api/create_general',headers:{'Content-Type':'application/x-www-form-urlencoded'},form:fields,body:fields?new URLSearchParams(fields).toString():null,curl:!fields||missing.length?null:`curl --request POST ${shell(url.origin+'/waf_rule/api/create_general')} \\\n  --cookie 'cookies.txt' \\\n  --header 'Content-Type: application/x-www-form-urlencoded' \\\n  --data-raw ${shell(new URLSearchParams(fields).toString())}`};
    });
  }
  function documentHTML(data) {
    return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>WAF 规则草案</title><style>body{font:14px/1.6 sans-serif;padding:24px;color:#24354a}table{border-collapse:collapse;width:100%}th{background:#d6e8f8}th,td{border:1px solid #d7dfe8;padding:10px;text-align:left;white-space:pre-wrap;overflow-wrap:anywhere;min-width:75px;vertical-align:top}td:nth-child(5){min-width:280px}li{white-space:pre-wrap}</style><h1>WAF 规则草案</h1><p>分数顺序：设置分数 / 严格阈值 / 普通阈值。ID 由目标系统分配；所有规则待人工复核。</p>${table(data)}<h2>分析说明</h2><ul>${data.notes.map(n=>`<li>${escape(n)}</li>`).join('')}</ul>${data.rules.map((r,i)=>`<h3>${i+1}. ${escape(r.name)}</h3><ul>${r.needs_review.map(n=>`<li>${escape(n)}</li>`).join('')}</ul>`).join('')}</html>`;
  }
  async function readStream(response, onText) {
    if(!response.ok) throw Error((await response.text()) || `HTTP ${response.status}`);
    if(!response.body) throw Error('服务未返回响应流');
    const reader=response.body.getReader(), decoder=new TextDecoder();
    let buffer='', text='', completed=false;
    function consume(final=false) {
      buffer=buffer.replace(/\r\n/g,'\n');
      let index;
      while((index=buffer.indexOf('\n\n'))>=0 || (final && buffer.length)) {
        if(index<0) index=buffer.length;
        const block=buffer.slice(0,index); buffer=buffer.slice(index+2);
        const raw=block.split('\n').filter(l=>l.startsWith('data:')).map(l=>l.slice(5).trimStart()).join('\n').trim();
        if(!raw) continue;
        if(raw==='[DONE]'){completed=true;continue;}
        const event=JSON.parse(raw);
        if(event.error) throw Error(event.error);
        text+=event.delta||event.content||'';
        if(text.length>2*1024*1024) throw Error('生成结果过大，请减少输入');
        onText(text);
      }
    }
    try {
      while(true) { const {value,done}=await reader.read(); if(done)break; buffer+=decoder.decode(value,{stream:true}); consume(); }
      buffer+=decoder.decode(); consume(true);
      if(!completed) throw Error('生成流提前结束，请重试');
      return text;
    } finally { await reader.cancel().catch(()=>{}); reader.releaseLock(); }
  }
  const api={columns,escape,validate,parse,cells,table,form,requests,documentHTML,readStream};
  if(typeof module!=='undefined') module.exports=api;
  else root.WafRules=api;
})(globalThis);
