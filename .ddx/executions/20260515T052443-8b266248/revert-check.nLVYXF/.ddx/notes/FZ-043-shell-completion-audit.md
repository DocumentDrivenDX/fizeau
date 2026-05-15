# FZ-043 shell completion audit

Bead: `agent-b8a80054`

Result: N/A. No checked-in shell completion scripts and no completion-generation
hooks are present, so this surface has no old command references to rename.

## Evidence

Checked-in script/path search:

```bash
rg --hidden --files \
  --glob '!.git/**' \
  --glob '!.ddx/**' \
  --glob '!.agents/**' \
  --glob '!.claude/**' \
  | rg '(^|/)(completion|completions|autocomplete|bash_completion|zsh|fish|_fiz|_agent|ddx-agent\.(bash|zsh|fish)|agent\.(bash|zsh|fish)|fiz\.(bash|zsh|fish))($|/|\.)'
```

Output: no matches, exit 1.

Completion generator/hook search:

```bash
rg --hidden -n 'Gen(Bash|Zsh|Fish|PowerShell)Completion|ShellCompDirective|ValidArgsFunction|RegisterFlagCompletionFunc|InitDefaultCompletionCmd|compdef|complete -F|complete -c|bash_completion' \
  --glob '!.git/**' \
  --glob '!.ddx/**' \
  --glob '!.agents/**' \
  --glob '!.claude/**' \
  --glob '!docs/**' \
  --glob '!testdata/**' \
  .
```

Output: no matches, exit 1.
