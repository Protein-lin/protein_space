(() => {
  const originalFetch = window.fetch.bind(window);
  window.fetch = (input, init = {}) => {
    const key = localStorage.getItem('waf_api_key');
    if (key) init.headers = { ...(init.headers || {}), 'X-API-Key': key };
    return originalFetch(input, init);
  };
  let register = false;
  const panel = () => document.querySelector('#authPanel');
  const message = text => { document.querySelector('#authMessage').textContent = text || ''; };
  document.querySelector('#authButton').onclick = () => panel().classList.remove('hidden');
  document.querySelector('#closeAuth').onclick = () => panel().classList.add('hidden');
  document.querySelector('#toggleAuth').onclick = () => { register = !register; document.querySelector('#authTitle').textContent = register ? '注册' : '登录'; document.querySelector('#authEmail').classList.toggle('hidden', !register); document.querySelector('#toggleAuth').textContent = register ? '已有账号？登录' : '没有账号？注册'; message(''); };
  document.querySelector('#authForm').onsubmit = async e => { e.preventDefault(); message(''); const payload = { username: document.querySelector('#authUsername').value, password: document.querySelector('#authPassword').value }; if (register) payload.email = document.querySelector('#authEmail').value; try { const res = await fetch(register ? '/api/auth/register' : '/api/auth/login', { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload) }); if (!res.ok) throw new Error(await res.text()); const user = await res.json(); document.querySelector('#authButton').textContent = user.username || '已登录'; panel().classList.add('hidden'); } catch (err) { message(err.message || '登录失败'); } };
})();
