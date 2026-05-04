## Reward (mean +/- SD across N reps)

| Harness | gpt-5-mini |
|---------|---------|
| ddx-agent | n/a |
| pi | n/a |
| opencode | n/a |

## Per-task pass count (out of N reps)

| Task | ddx-agent / gpt-5-mini | opencode / gpt-5-mini | pi / gpt-5-mini |
|------|---------|---------|---------|
| fix-git | 0/3 | 0/3 | 0/3 |
| git-leak-recovery | 0/3 | 0/3 | 0/3 |
| log-summary-date-ranges | 0/3 | 0/3 | 0/3 |

## Costs

| Cell | Input tok | Output tok | Cached tok | Retried tok | Cost ($) |
|------|-----------|------------|------------|-------------|----------|
| ddx-agent / gpt-5-mini | 0 | 0 | 0 | 0 | 0.000000 |
| opencode / gpt-5-mini | 0 | 0 | 0 | 0 | 0.000000 |
| pi / gpt-5-mini | 0 | 0 | 0 | 0 | 0.000000 |

## Non-graded runs

| Cell / rep / task | final_status | cause |
|-------------------|--------------|-------|
| ddx-agent / gpt-5-mini / 1 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| ddx-agent / gpt-5-mini / 1 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| ddx-agent / gpt-5-mini / 1 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| ddx-agent / gpt-5-mini / 2 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| ddx-agent / gpt-5-mini / 2 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| ddx-agent / gpt-5-mini / 2 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| ddx-agent / gpt-5-mini / 3 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| ddx-agent / gpt-5-mini / 3 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| ddx-agent / gpt-5-mini / 3 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| opencode / gpt-5-mini / 1 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| opencode / gpt-5-mini / 1 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| opencode / gpt-5-mini / 1 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| opencode / gpt-5-mini / 2 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| opencode / gpt-5-mini / 2 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| opencode / gpt-5-mini / 2 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| opencode / gpt-5-mini / 3 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| opencode / gpt-5-mini / 3 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| opencode / gpt-5-mini / 3 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| pi / gpt-5-mini / 1 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| pi / gpt-5-mini / 1 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| pi / gpt-5-mini / 1 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| pi / gpt-5-mini / 2 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| pi / gpt-5-mini / 2 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| pi / gpt-5-mini / 2 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
| pi / gpt-5-mini / 3 / fix-git | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/fix-git |
| pi / gpt-5-mini / 3 / git-leak-recovery | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/git-leak-recovery |
| pi / gpt-5-mini / 3 / log-summary-date-ranges | install_fail_permanent | task directory not found: /tmp/ddx-exec-wt/.execute-bead-wt-agent-5b6f5872-20260504T044505-f74b7e58/scripts/benchmark/external/terminal-bench-2/tasks/log-summary-date-ranges |
