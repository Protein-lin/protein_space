// Shared by chat and the WAF workbench on this browser/site.
(function(root) {
  const key = 'protein_last_requested_model';
  function remember(model) {
    if (typeof model !== 'string' || !model) return;
    try { localStorage.setItem(key, model); } catch { /* Storage may be disabled. */ }
  }
  function choose(models, fallback, conversationModel) {
    let saved = '';
    try { saved = localStorage.getItem(key) || ''; } catch { /* Use available defaults. */ }
    const available = new Set(models.map(model => model.id));
    // Resuming an existing conversation keeps its model if still permitted.
    return [conversationModel, saved, fallback, models[0]?.id].find(id => id && available.has(id)) || '';
  }
  root.ModelPreference = {remember, choose};
})(globalThis);
