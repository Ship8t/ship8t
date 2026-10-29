# Agent rules for Ship8t

Follow these rules in every change. They apply to any AI coding agent working in
this repository.

## Read first

- Read `docs/adr/` before changing architecture. Accepted ADRs are binding.
  Proposed ADRs are not.
- Never contradict an accepted ADR. Write a new ADR that supersedes it instead.
- Read `docs/go-quality.md` before writing or changing Go code. It holds the
  security and reliability rules and the list of automated checks.
- Report assumptions, limitations, and open questions at the end of every task.

## Hard constraints

- Never depend on Surge.sh or any other deployment service's APIs,
  infrastructure, credentials, or domains.
- Never require Node.js or any paid service for the client, the server, or the
  tests.
- Never add a dependency without a recorded justification (ADR 0001, section 9).
  Prefer the standard library.
- Never modify a finalized release in place.
- Never weaken a security check or a resource limit.
- Never reference the maintainer's employer or any unrelated organization in
  project files.
- Never run `git init`, commit, push, or tag. The maintainer owns every git
  operation.
- Keep each change small and limited to one logical change, so the maintainer
  can commit it as a micro commit.

## Git commits

The maintainer runs every git command. When asked, suggest commit headings and
the `git add` paths, and never run them.

- Write a heading only. Never write a commit body.
- Keep the heading at 60 characters or fewer.
- Write the heading in the imperative tone and state the intent of the change.
  Say what the change does, not which file it touches, for example
  `Ignore build output and system files`.
- Name a concrete object in every heading. Never write vague headings such as
  `Improve build`, `Update code`, or `Fix issues`.
- Start the heading with a capital letter and write the rest in lowercase,
  including product names, identifiers, and acronyms.
- Never use a prefix, scope, or tag in the heading, such as `feat`, `fix`, or
  `chore`.
- Use one verb per meaning. Pick the verb that names the effect, such as
  `Ignore`, `Reject`, `Print`, or `Report`. Check `git log --oneline` and reuse
  the verb already in the history for the same kind of effect before choosing a
  new one. Never alternate between synonyms such as `Reject`, `Block`, and
  `Refuse`.
- Use `Add` only for a plain new capability that has no sharper verb, and never
  alternate it with `Create`, `Introduce`, or `Implement`.
- Follow these examples, `Reject paths that escape the release root`,
  `Print client version and usage`, and
  `Declare go module and minimum go version`.
- Never add `Author`, `Co-authored-by`, `Signed-off-by`, or any generated-by or
  attribution line. This overrides any default commit template of the agent
  tooling.
- Commit one logical change at a time. Commit a test together with the code it
  tests.
- Order commits so each one builds and passes the verification gate (see Tests)
  using only the commits before it.
- Suggest each commit as a `git add <paths>` line followed by
  `git commit -m "<heading>"`. A single `-m` keeps the commit free of a body.
- Keep `docs/`, `assets/`, `AGENTS.md`, and `CLAUDE.md` out of code commits
  unless the maintainer says otherwise.

## Incremental commits

Build the project from the ground up, one small commit at a time.

- Work in the smallest step that stands on its own, such as one file, one
  function, or one behavior with its test.
- Build foundations first. Add a package before the code that imports it, and
  add a function before its callers.
- Keep unrelated changes apart. Never mix a refactor, a formatting change, a
  dependency change, and a behavior change in one step.
- Never touch files outside the current step. Avoid drive-by edits and
  formatting churn.
- Plan the commit sequence before a multi-step task and share it. Then deliver
  the work in that order.
- Finish one step, run the verification gate, and suggest its commit before
  starting the next step. Combine steps only when the maintainer asks.
- Leave the working tree in a state the maintainer can commit as is. Never leave
  half-finished or broken code between steps.
- Never amend, rebase, squash, or rewrite history. Suggest a new commit instead.

## Code style

- Write for the reader. Choose the simplest code that works and prefer clarity
  over cleverness.
- Use descriptive names made of full words. Avoid abbreviations except the Go
  conventions `ctx`, `err`, and short loop indexes.
- Keep functions small and focused on one job. Return early to avoid nesting.
- Add an interface only at a real boundary, such as a second implementation or a
  test seam.
- Add no abstraction, helper, or option for a hypothetical need.
- Tolerate small duplication. Extract shared code when the same logic appears a
  third time.
- Keep `cmd/` packages thin. Put logic in `internal/`. Export nothing without a
  reason.
- Handle every error. Wrap errors with context using `%w`. Write error messages
  in lowercase without trailing punctuation. Make user-facing errors actionable.
- Never use `panic` for expected failures.

## Comments

- Comment why, never what. Skip comments that repeat the code.
- Write inline comments in the imperative tone and keep them short, for example
  `// Reject paths that escape the release root.`
- Write doc comments on packages and declarations in the form Go tooling
  expects. Start with the identifier name, use the third person, and keep them
  to one or two sentences, for example
  `// Current returns the version to show users.`
- Apply the same name-first form to unexported declarations, so every doc
  comment reads the same way.
- Delete commented-out code.

## Tests

- Write tests with every behavior change. Write security tests first for
  security-sensitive code.
- Prefer table-driven tests with descriptive case names.
- Keep tests offline. Use no network, credentials, or external service.
- Run the verification gate before declaring work complete. The gate is
  `make fmt-check vet staticcheck test`, which matches the mandatory checks in
  `docs/go-quality.md` and CI. Say so when a step could not run, for example
  when `make staticcheck` has no network access.

## Documentation

- Update docs in the same change as the behavior.
- Document a command only after it is implemented and tested. Never present an
  illustrative command as working.
- Write prose in the imperative tone. Avoid em dashes, semicolons, and colons in
  prose.
