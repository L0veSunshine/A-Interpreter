# XLang Syntax

VS Code syntax highlighting and completion for `.xl` files. Completion includes
keywords, built-in functions, methods, and declarations in the current file.
Method suggestions are not type-aware. Diagnostics are not provided.

To try it, open this folder in VS Code and press F5. In the Extension
Development Host, open an `.xl` file.

To install it locally, run `npx @vscode/vsce package` in this folder, then
use **Extensions: Install from VSIX...** in VS Code and select the generated
`.vsix` file.

Run `npm install` and `npm test` to validate the grammar and completion provider.