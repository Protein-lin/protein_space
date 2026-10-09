const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
function createPage(api, wx = { getStorageSync() {}, setStorageSync() {} }) {
  let definition;
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../pages/chat/chat.js'), 'utf8'), {
    require: () => api, Page(value) { definition = value; }, wx, Date, Math
  });
  const page = { ...definition, data: JSON.parse(JSON.stringify(definition.data)), updates: 0 };
  page.setData = function(values, callback) {
    this.updates++;
    for (const [key, value] of Object.entries(values)) {
      const match = key.match(/^messages\[(\d+)\]\.content$/);
      if (match) this.data.messages[Number(match[1])].content = value;
      else this.data[key] = value;
    }
    if (callback) callback();
  };
  page.onLoad();
  return page;
}
module.exports = { createPage };
