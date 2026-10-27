# Architecture Decision Records

This directory records the significant architectural decisions for Ship8t. Each ADR explains the context, the alternatives, the decision, and the consequences.

## Status vocabulary

| Status | Meaning |
|---|---|
| Proposed | Written and open for review. Not binding. Do not build against it as if it were final. |
| Accepted | The maintainer accepted it. Code and docs must follow it. |
| Superseded | A later ADR replaced it, and the old ADR links to the new one. |
| Deprecated | No longer relevant, with no replacement. |

Never edit an accepted ADR silently. To change a decision, write a new ADR that supersedes the old one and update the old ADR's status to link to it. Record small clarifications in the ADR's history section.

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-v1-scope-and-architecture.md) | Ship8t v1 scope and architecture | Accepted |
| [0002](0002-deployment-states-and-activation.md) | Deployment states and activation | Accepted |
| [0003](0003-storage-and-atomic-publish.md) | Storage and atomic publish | Accepted |
| [0004](0004-api-and-upload-protocol.md) | API and upload protocol | Accepted |
| [0005](0005-minimum-go-version.md) | Minimum Go version | Accepted |

## Topics without an ADR yet

Write an ADR for each of these when implementation shows that 0001 to 0004 are not enough.

- A detailed threat model. The v1 security boundaries live in 0001, section 10.
- Remote authentication beyond static scoped tokens.
- Public hostnames and TLS.
- Cross-release file reuse and custom response headers.

## Writing a new ADR

1. Copy [template.md](template.md) to `NNNN-short-title.md`, replacing `NNNN` with the next free four-digit number.
2. Fill in the Status, Date, and Deciders lines and every section. Label options that are still undecided as proposed.
3. Open it as Proposed and add it to the index above.
4. Mark it Accepted, with a date, only when the maintainer accepts it.

Name each file after its subject in plain words, such as `0005-security-threat-model.md`.
