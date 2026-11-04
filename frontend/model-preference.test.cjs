const test=require('node:test');
const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');
const source=fs.readFileSync(__dirname+'/model-preference.js','utf8');
const models=[{id:'a'},{id:'b'}];
function setup(storage=new Map()) {
  const context=vm.createContext({localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v)}});
  vm.runInContext(source,context);return context.ModelPreference;
}
test('last requested model persists across page reloads and overrides server default',()=>{
  const storage=new Map(), first=setup(storage);
  assert.equal(first.choose(models,'a'),'a');
  first.remember('b');
  assert.equal(setup(storage).choose(models,'a'),'b');
});
test('unavailable saved model falls back to permitted server default or first model',()=>{
  const preference=setup();preference.remember('removed');
  assert.equal(preference.choose(models,'b'),'b');
  assert.equal(preference.choose(models,'removed'),'a');
  assert.equal(preference.choose([],'removed'),'');
});
test('existing conversation keeps its model without changing last requested preference',()=>{
  const preference=setup();preference.remember('b');
  assert.equal(preference.choose(models,'a','a'),'a');
  assert.equal(preference.choose(models,'a'),'b');
  assert.equal(preference.choose(models,'a','removed'),'b');
});
test('disabled browser storage cannot prevent requests or model loading',()=>{
  const context=vm.createContext({localStorage:{getItem(){throw Error('blocked')},setItem(){throw Error('blocked')}}});
  vm.runInContext(source,context);
  assert.doesNotThrow(()=>context.ModelPreference.remember('b'));
  assert.equal(context.ModelPreference.choose(models,'a'),'a');
});
