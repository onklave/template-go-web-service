# Project rule book

These are the guardrails an agent must follow when working this project's issues.
This is a starting point — edit and extend it with your own rules.

## Source control
- **Never push to the source-of-truth branch.** Work reaches it only through a
  reviewed pull request.
- **The platform owns branching and the pull request.** You start on a work
  branch already cut for the item — commit there. Don't create or switch
  branches, don't push, and don't open the PR yourself: the platform does that
  once the gates pass, targeting the project's source-of-truth branch.
- Keep each PR scoped to the one issue it addresses.
- The PR description must be informative: what the problem was, the root cause,
  what changed and why, how it was verified, and the options chosen (model,
  execution mode, any rules applied).

## How to work
1. **Think before coding.** State your assumptions. If the issue is ambiguous,
   surface the interpretations rather than guessing.
2. **Simplicity first.** Write the minimum code that solves the problem — no
   speculative abstractions, no unrequested features.
3. **Surgical changes.** Touch only what's necessary; match the surrounding
   style. Don't refactor unrelated code. Every changed line should trace to the
   issue.
4. **Verify before opening a PR.** Run the project's lint, tests, and build.
   Don't open a PR on red.
5. **Leave every file you write already formatted.** Run the repo's formatter
   (whatever `format`, `lint:fix` or `code:fix` script it provides) before you
   commit, and end every file with a trailing newline. This covers generated
   data and docs, not only source. Where a repo's release pipeline formats as
   one of its own steps, an unformatted file dirties the tree mid-run and
   stalls the release — and it surfaces as an unrelated dirty-tree error, not
   as a formatting problem, so it costs far more to diagnose than to avoid.

## Building and deploying
The repo declares; the platform executes. `onklave.yaml` at the repo root is the
one file the platform reads to learn what to build, run and route.

- **A new deployable is a new entry under `services`** — its own `build.context`
  and `build.dockerfile`, its `runtime.port` and `runtime.healthPath`, and its
  `expose` route. Adding a backend beside a front end means editing this file.
- **Anything not declared there is not built and not deployed.** There is no
  other place to say "this also needs building".
- **Never add a CI workflow to make something build.** The platform builds
  in-cluster, runs the tests through its own gates, scans the image and
  reconciles the result. A `.github/workflows` file is run by nothing here, and
  the platform's credential cannot even push one.
- `build.context` must be a directory that exists in the repo. A context that
  isn't there fails the build outright — it is not treated as a hint.
- **A route prefix is not stripped.** A service exposed at `/api` is asked for
  `/api/...`, and so is its health probe: declare `healthPath: /api/healthz`,
  not `/healthz`, or the probe 404s and the rollout fails.
- Exactly one service may claim `/`. Exposed services must agree on
  `expose.auth` — the environment is served on one host and the gate is a
  property of that host, not of a path.
- A service with no inbound port sets `expose.enabled: false` (workers, internal
  APIs). It is then reachable only from inside the app's own namespace.

## Quality bar
- Add or update tests for the behaviour you change.
- Don't introduce new dependencies without flagging it.
- Don't weaken security, auth, or input validation to make something pass.
- If you cannot solve the issue, or it needs a human decision, escalate it for
  human investigation rather than forcing a low-confidence change.
- When you need a decision or missing information from a human, propose a
  `support` follow-up carrying explicit `questions` — the platform collects
  the answers before the task is created, so never file a task that just says
  "ask the owner". Support tasks are answered by humans, not run by agents.