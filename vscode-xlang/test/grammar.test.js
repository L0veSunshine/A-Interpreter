const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const textmate = require('vscode-textmate');
const oniguruma = require('vscode-oniguruma');

async function main() {
  const wasm = fs.readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm'));
  await oniguruma.loadWASM(wasm.buffer.slice(wasm.byteOffset, wasm.byteOffset + wasm.byteLength));
  const grammarPath = path.join(__dirname, '../syntaxes/xlang.tmLanguage.json');
  const registry = new textmate.Registry({
    onigLib: Promise.resolve({
      createOnigScanner: patterns => new oniguruma.OnigScanner(patterns),
      createOnigString: source => new oniguruma.OnigString(source)
    }),
    loadGrammar: async scope => scope === 'source.xlang'
      ? textmate.parseRawGrammar(fs.readFileSync(grammarPath, 'utf8'), grammarPath)
      : null
  });
  const grammar = await registry.loadGrammar('source.xlang');

  function check(line, text, scope, initialState = null) {
    const result = grammar.tokenizeLine(line, initialState);
    const start = line.indexOf(text);
    assert.notEqual(start, -1, text);
    const token = result.tokens.find(item => item.startIndex <= start && item.endIndex >= start + text.length);
    assert.ok(token && token.scopes.includes(scope), `${JSON.stringify(text)} should have scope ${scope}`);
    return result.ruleStack;
  }

  check('def 求值(x) {', '求值', 'entity.name.function.xlang');
  check('var 变量 = 0xFF + .5e-2', 'var', 'storage.type.xlang');
  check('var 变量 = 0xFF + .5e-2', '0xFF', 'constant.numeric.xlang');
  check('var 变量 = 0xFF + .5e-2', '.5e-2', 'constant.numeric.xlang');
  check('a //= 2 # comment', '//=', 'keyword.operator.xlang');
  check('a //= 2 # comment', '# comment', 'comment.line.number-sign.xlang');
  check('print("# // \\n")', '# // ', 'string.quoted.double.xlang');
  check('print("# // \\n")', '\\n', 'constant.character.escape.xlang');
  check('print(1)', 'print', 'support.function.builtin.xlang');
  const identifier = grammar.tokenizeLine('变量var = 1', null).tokens[0];
  assert.ok(!identifier.scopes.includes('storage.type.xlang'));
  const stringState = check('"first', 'first', 'string.quoted.double.xlang');
  check('second"', 'second', 'string.quoted.double.xlang', stringState);
  console.log('XLang grammar tests passed');
}

main().catch(error => { console.error(error); process.exitCode = 1; });