const keywords = ['var', 'for', 'def', 'if', 'else', 'return', 'break', 'true', 'false', 'none', 'and', 'or', 'not'];
const builtins = [
  ['print', 'value'], ['len', 'value'], ['type', 'value'], ['int', 'value'], ['float', 'value']
];
const methods = [
  ['append', 'value', 'Array'], ['pop', '', 'Array'], ['reverse', '', 'Array'],
  ['index', 'value', 'Array'], ['split', 'separator', 'String'],
  ['upper', '', 'String'], ['lower', '', 'String']
];
const snippets = {
  var: 'var ${1:name} = ${2:value}$0',
  def: 'def ${1:name}(${2:args}){\n\t$0\n}',
  if: 'if(${1:condition}){\n\t$0\n}',
  else: 'else{\n\t$0\n}',
  for: 'for(var ${1:i} = 0; ${1:i} < ${2:n}; ${1:i} += 1){\n\t$0\n}'
};

function maskSource(source) {
  let state = 'code';
  let escaped = false;
  let masked = '';
  for (const character of source) {
    if (state === 'comment') {
      if (character === '\n' || character === '\r') {
        state = 'code';
        masked += character;
      } else {
        masked += ' '.repeat(character.length);
      }
    } else if (state !== 'code') {
      masked += character === '\n' || character === '\r' ? character : ' '.repeat(character.length);
      if (escaped) escaped = false;
      else if (character === '\\') escaped = true;
      else if (character === state) state = 'code';
    } else if (character === '#') {
      state = 'comment';
      masked += ' ';
    } else if (character === '"' || character === "'") {
      state = character;
      masked += ' ';
    } else {
      masked += character;
    }
  }
  return { masked, state };
}

function createCompletionProvider(vscode) {
  return {
    provideCompletionItems(document, position) {
      const source = document.getText();
      const offset = document.offsetAt(position);
      const before = maskSource(source.slice(0, offset));
      if (before.state !== 'code') return [];

      const line = document.lineAt(position.line).text;
      const prefix = line.slice(0, position.character).match(/[\p{L}_0-9]*$/u)[0];
      const suffix = line.slice(position.character).match(/^[\p{L}_0-9]*/u)[0];
      const range = new vscode.Range(position.line, position.character - prefix.length, position.line, position.character + suffix.length);
      const member = before.masked.slice(0, offset - prefix.length).endsWith('.');
      const next = line.slice(position.character + suffix.length).trimStart();
      const items = [];

      function add(name, kind, detail, argument, snippet) {
        const item = new vscode.CompletionItem(name, kind);
        item.range = range;
        if (detail) item.detail = detail;
        if (snippet) item.insertText = new vscode.SnippetString(snippet);
        else if (argument !== undefined && !next.startsWith('(')) {
          item.insertText = new vscode.SnippetString(argument ? `${name}(\${1:${argument}})$0` : `${name}()$0`);
        }
        items.push(item);
      }

      if (member) {
        for (const [name, argument, type] of methods) add(name, vscode.CompletionItemKind.Method, `${type} method`, argument);
        return items;
      }

      for (const name of keywords) {
        const snippet = next.startsWith('(') && ['for', 'if'].includes(name) ? undefined : snippets[name];
        add(name, vscode.CompletionItemKind.Keyword, 'XLang keyword', undefined, snippet);
      }
      for (const [name, argument] of builtins) add(name, vscode.CompletionItemKind.Function, 'XLang built-in function', argument);

      const masked = maskSource(source).masked;
      const seen = new Set([...keywords, ...builtins.map(([name]) => name)]);
      const declarations = /(?<![\p{L}_0-9])(var|def)\s+([\p{L}_][\p{L}_0-9]*)/gu;
      for (const match of masked.matchAll(declarations)) {
        const name = match[2];
        if (seen.has(name)) continue;
        seen.add(name);
        add(name, match[1] === 'def' ? vscode.CompletionItemKind.Function : vscode.CompletionItemKind.Variable, 'Current file');
      }
      const functions = /(?<![\p{L}_0-9])def\s+[\p{L}_][\p{L}_0-9]*\s*\(([^)]*)\)/gu;
      for (const match of masked.matchAll(functions)) {
        for (const part of match[1].split(',')) {
          const name = part.trim();
          if (!/^[\p{L}_][\p{L}_0-9]*$/u.test(name) || seen.has(name)) continue;
          seen.add(name);
          add(name, vscode.CompletionItemKind.Variable, 'Function parameter');
        }
      }
      return items;
    }
  };
}

function activate(context) {
  const vscode = require('vscode');
  context.subscriptions.push(vscode.languages.registerCompletionItemProvider('xlang', createCompletionProvider(vscode), '.'));
}

module.exports = { activate, createCompletionProvider };