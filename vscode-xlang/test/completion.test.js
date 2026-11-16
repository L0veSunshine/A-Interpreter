const assert = require('node:assert/strict');
const { createCompletionProvider } = require('../extension');

class CompletionItem {
  constructor(label, kind) { this.label = label; this.kind = kind; }
}
class SnippetString {
  constructor(value) { this.value = value; }
}
class Range {
  constructor(startLine, start, endLine, end) { Object.assign(this, { startLine, start, endLine, end }); }
}
const vscode = { CompletionItem, SnippetString, Range, CompletionItemKind: { Keyword: 1, Function: 2, Variable: 3, Method: 4 } };
const provider = createCompletionProvider(vscode);

function complete(source, marker = '|') {
  const offset = source.indexOf(marker);
  assert.notEqual(offset, -1);
  const text = source.slice(0, offset) + source.slice(offset + marker.length);
  const lines = text.split('\n');
  const before = text.slice(0, offset).split('\n');
  const position = { line: before.length - 1, character: before[before.length - 1].length };
  return provider.provideCompletionItems({
    getText: () => text,
    offsetAt: () => offset,
    lineAt: index => ({ text: lines[index] })
  }, position);
}

let items = complete('var 队列 = []\ndef 搜索(起点){\n  var x = 1\n}\n|');
for (const name of ['var', 'for', 'len', '队列', '搜索', '起点', 'x']) assert.ok(items.some(item => item.label === name), name);
assert.ok(items.find(item => item.label === 'for').insertText.value.includes('for(var'));
assert.equal(items.find(item => item.label === 'len').insertText.value, 'len(${1:value})$0');
items = complete('队列.a|');
assert.ok(items.some(item => item.label === 'append'));
assert.ok(items.some(item => item.label === 'split'));
assert.ok(!items.some(item => item.label === 'var'));
assert.equal(items.find(item => item.label === 'append').range.start, 3);
assert.equal(complete('len|(').find(item => item.label === 'len').insertText, undefined);
assert.equal(complete('for|(').find(item => item.label === 'for').insertText, undefined);
assert.equal(complete('if|(').find(item => item.label === 'if').insertText, undefined);
assert.deepEqual(complete('# var 隐藏\n"|"'), []);
assert.deepEqual(complete('# |'), []);
assert.deepEqual(complete('"first\n|second"'), []);
assert.ok(!complete('# var 隐藏\n|').some(item => item.label === '隐藏'));
console.log('XLang completion tests passed');