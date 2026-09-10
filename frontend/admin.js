(() => {
  const button = document.querySelector('#adminButton');
  const panel = document.querySelector('#adminPanel');
  const content = document.querySelector('#adminContent');
  const message = document.querySelector('#adminMessage');
  if (!button || !panel) return;
  const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const api = async (action, payload = {}) => {
    const response = await fetch('/api/admin/config', {method: action ? 'POST' : 'GET', headers: {'Content-Type':'application/json'}, body: action ? JSON.stringify({action, ...payload}) : undefined});
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  };
  const show = text => { message.textContent = text || ''; };
  const render = data => {
    content.innerHTML = `<div class="admin-grid">
      <section><h3>IP 白名单</h3><form data-action="whitelist_add"><input name="cidr" required placeholder="IP 或 CIDR，如 1.2.3.4/32"><input name="description" placeholder="说明"><button>添加</button></form><div class="admin-list">${data.whitelist.map(x => `<div><code>${esc(x.cidr)}</code> ${esc(x.description)} <button data-delete="whitelist_delete" data-id="${x.id}">删除</button></div>`).join('')}</div></section>
      <section><h3>用户</h3><form data-action="user_add"><input name="username" required minlength="3" placeholder="用户名"><input name="email" type="email" placeholder="邮箱"><input name="password" required minlength="8" type="password" placeholder="初始密码"><button>创建用户</button></form><div class="admin-list">${data.users.map(x => `<div><b>${esc(x.username)}</b> · ${esc(x.email)} · ${esc(x.status)} <button data-status="${x.id}" data-value="${x.status === 'active' ? 'disabled' : 'active'}">${x.status === 'active' ? '禁用' : '启用'}</button></div>`).join('')}</div></section>
      <section><h3>Agent / Provider 配置</h3><form data-action="provider_add"><input name="name" required placeholder="配置名称"><input name="base_url" required placeholder="https://api.example.com/v1/chat/completions"><input name="api_key" required type="password" placeholder="API Key"><input name="default_model" required value="gpt-5.5" placeholder="默认模型"><button>保存配置</button></form><div class="admin-list">${data.providers.map(x => `<div><b>${esc(x.name)}</b> · ${esc(x.base_url)} · ${esc(x.default_model)} · ${esc(x.api_key_hint || '已加密')} <button data-provider-status="${x.id}" data-value="${x.enabled ? 'false' : 'true'}">${x.enabled ? '禁用' : '启用'}</button> <button data-delete="provider_delete" data-id="${x.id}">删除</button></div>`).join('')}</div></section>
    </div>`;
  };
  const refresh = async () => { show('加载中…'); try { render(await api()); show(''); } catch (error) { show(error.message || '无权访问配置页'); } };
  button.onclick = () => { panel.classList.remove('hidden'); refresh(); };
  document.querySelector('#closeAdmin').onclick = () => panel.classList.add('hidden');
  panel.addEventListener('submit', async event => {
    event.preventDefault(); const form = event.target; const payload = Object.fromEntries(new FormData(form));
    try { await api(form.dataset.action, payload); await refresh(); } catch (error) { show(error.message); }
  });
  panel.addEventListener('click', async event => {
    const target = event.target;
    try {
      if (target.dataset.delete) { await api(target.dataset.delete, {id: Number(target.dataset.id)}); await refresh(); }
      if (target.dataset.status) { await api('user_status', {id: Number(target.dataset.status), status: target.dataset.value}); await refresh(); }
      if (target.dataset.providerStatus) { await api('provider_status', {id: Number(target.dataset.providerStatus), enabled: target.dataset.value === 'true'}); await refresh(); }
    } catch (error) { show(error.message); }
  });
  fetch('/api/admin/config').then(response => { if (response.ok) button.classList.remove('hidden'); }).catch(() => {});
})();
