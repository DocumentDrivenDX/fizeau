[Terminal-Bench](https://terminal-bench.dev/) 2.1 is a public coding-agent benchmark of 89 long-form tasks. Each task ships a prompt, an isolated Docker environment, and a deterministic verifier. An agent reads the prompt, runs shell commands, edits files inside the container, and is scored against the resulting state.

Each Fizeau lane runs through [Harbor](https://github.com/laude-institute/harbor) 0.3.x's installed-agent path. Harbor installs the agent runtime in the task container, runs the attempt, and then invokes the verifier separately. Lane configuration selects the provider, model, runtime, and harness without publishing private service locations. Each task runs five reps per lane; pass@1 is the per-rep success rate, and pass@k reports whether any of the five reps solved the task.

We slice the 89-task set into nested benchmarks of decreasing scope. The subset YAMLs are under `scripts/benchmark/task-subset-tb21-*.yaml`:
