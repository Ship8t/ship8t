# 0002. Deployment states and activation

- **Status** Accepted
- **Date** 2026-10-26
- **Deciders** Ryan Jay Leyva

## Context and problem

The product plan lists the deployment states `pending`, `uploading`, `validating`, `ready`, `published`, `failed`, and `cancelled`. It also says a superseded release may be recorded separately from its immutable history, and it requires rollback to stay safe when a newer deployment races it.

That list has two problems.

1. `published` mixes two facts. One is what a deployment is, a finalized and immutable release. The other is whether it is serving traffic right now. A release that was published, replaced, and rolled back to would have to leave `published` and re-enter it, which contradicts immutability.
2. The plan requires defined behavior for publish ordering, repeated requests, and rollback races, and it does not specify any.

## Requirements and constraints

- Never change a finalized release or its recorded state in place.
- Make publishing and rollback atomic, and never expose incomplete content.
- Never let a failed or interrupted deployment change what is active.
- Reject invalid state transitions and cover them with automated tests.
- Make repeated requests safe, so client retries never corrupt state.
- Give concurrent publishes and rollbacks within one project a deterministic outcome.
- Keep the model small. Add no state that exists only for a hypothetical future need.

## Alternatives considered

- **Keep `published` as a deployment state, as the plan does.** Easy to read, but rollback must either rewrite a deployment's state or copy the old release. Both weaken immutability and muddy history. Rejected.
- **Deployment states plus a separate active pointer.** Deployment state describes readiness only. A per-project pointer names the active deployment, and an append-only log records every pointer change. Chosen.
- **Event sourcing for all state.** Powerful, but far more machinery than v1 needs, since only the activation history must be append-only. Rejected.
- **A separate `validating` state.** Validation is cheap (confirm every manifest file is present with the right size and checksum) and runs inside `finalize`. A separate state matters only if validation becomes asynchronous. Rejected for v1. Add it through a superseding ADR if that changes.

## Decision

### Deployment states

| State | Meaning | Terminal |
|---|---|---|
| `created` | The record and manifest exist. No content has been accepted. | No |
| `uploading` | At least one file has been accepted. | No |
| `ready` | Finalized. Content is complete, verified, and immutable. Eligible for activation. | No |
| `failed` | Finalization failed in a way that retrying cannot repair. | Yes |
| `cancelled` | The client cancelled the deployment. | Yes |
| `expired` | The deployment was abandoned before finalization and cleaned up after the idle timeout. | Yes |
| `pruned` | Retention removed the content of a `ready` release. The record stays for history. | Yes |

`pruned` extends the plan's list. Without it, history cannot tell a deployment that never finished from one that finished and was later deleted, and rollback cannot tell which releases still have content.

### Allowed transitions

| From | To | Trigger |
|---|---|---|
| `created` | `uploading` | The first file is accepted. |
| `created`, `uploading` | `ready` | `finalize` succeeds with every file present and verified. |
| `created`, `uploading` | `failed` | `finalize` hits an unrecoverable error, such as a staged file failing re-verification or an I/O error. |
| `created`, `uploading` | `cancelled` | The client cancels. |
| `created`, `uploading` | `expired` | The deployment sees no activity for the idle timeout (default 1 hour). |
| `ready` | `pruned` | Retention removes the release. Never allowed for the active release. |

Reject every other transition with `409 state_conflict`. Never change a terminal state.

Treat a request that would produce the current state as a successful no-op. `finalize` on a `ready` deployment and `cancel` on a `cancelled` deployment both succeed, which makes client retries safe.

A `finalize` call on a deployment with missing files is not a failure. Return `409` with the list of missing files and leave the deployment in `uploading`, so the client uploads the rest and calls `finalize` again.

### Activation

- Give each project at most one active deployment, and none before the first publish.
- Append a row to the activation log on every change. Each row holds the project, the deployment, the previous deployment, the kind (`publish` or `rollback`), the actor, and a timestamp. Never update or delete a row.
- Update the project's active pointer in the same database transaction that appends the log row (see [ADR 0003](0003-storage-and-atomic-publish.md)).
- Publish activates a `ready` deployment. Rollback activates an earlier `ready` deployment that is not `pruned`. Both change only the pointer and never touch a deployment's state or content.
- Derive "superseded" from the log, meaning a deployment that was active and no longer is. Never store it.
- Treat publishing the already-active deployment as a successful no-op that writes no log row.
- Default the rollback target to the previous deployment of the latest activation. If it is `pruned`, fail with `409` and require the caller to name a target. Rolling back twice in a row returns to the original release, so deeper rollbacks name an explicit target.

### Ordering and races

- Serialize activation operations per project with database write transactions.
- Let the last committed activation win. A deployment that started earlier but publishes later wins, because commit order is the order of record.
- Protect races with `expected_active=<id>`, exposed as `--if-active=<id>` in the client. The server applies the activation only when the active deployment still matches. Otherwise it returns `409 active_mismatch` and changes nothing. A rollback that races a newer publish either lands before it or fails cleanly.
- Make the client's default `deploy` run create, upload, finalize, and publish. `--no-publish` stops at `ready`.

## Consequences and trade-offs

- Rollback is cheap and honest. It copies nothing, rewrites nothing, and shows exactly what happened in history.
- Users learn two concepts, deployment state and activation, and the two never overlap.
- Last commit wins is simple but lets a slow deploy overwrite a newer one. The `--if-active` guard and a documented warning address that. A stricter rule that rejects publishing anything older than the active release can come later through a superseding ADR.
- Pruning can remove a rollback target. Retention (ADR 0003) and the explicit-target rule bound that risk.

## Implementation implications

- Route every state change through one transition function that validates `(from, to)` against the table above. Cover every allowed and every rejected transition in table-driven tests.
- Test idempotent repeats, `finalize` with missing files, publishing a deployment that is not `ready`, rollback to a pruned release, an `expected_active` mismatch, and two concurrent publishes.
- Expose deployment state and the activation log separately in the API (ADR 0004).
- Write user documentation that calls a deployment "active", never "published".

## Status and history

- 2026-10-26. Accepted, together with ADRs 0001, 0003, and 0004.
- 2026-10-26. Renumbered from 0001 to 0002 and reworded before any implementation. No decision changed.
