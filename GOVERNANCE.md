# Governance

This document describes the governance model for the **kindling** project.

## Principles

- **Open** — All discussions, decisions, and code happen in public.
- **Transparent** — Roadmap, meeting notes, and design proposals are accessible to everyone.
- **Merit-based** — Contributions and sustained involvement earn increased responsibility.

## Roles

### Users

Anyone who uses kindling. Users are encouraged to participate by filing issues, joining discussions, and providing feedback.

### Contributors

Anyone who contributes code, documentation, tests, or other improvements via pull requests. Contributors must sign off their commits under the [Developer Certificate of Origin (DCO)](https://developercertificate.org/).

### Reviewers

Contributors who have demonstrated sustained, high-quality contributions may be invited to become Reviewers. Reviewers can approve pull requests and are listed in the [MAINTAINERS.md](MAINTAINERS.md) file.

**Becoming a Reviewer:**
- Sustained contributions over 2+ months
- Demonstrated understanding of the codebase
- Nominated by an existing Maintainer
- Approved by lazy consensus (no objection within 7 days)

### Maintainers

Maintainers have write access to the repository and are responsible for:
- Approving and merging pull requests
- Triaging issues
- Cutting releases
- Upholding the project's technical direction

**Becoming a Maintainer:**
- Active Reviewer for 3+ months
- Demonstrated leadership in technical discussions
- Nominated by an existing Maintainer
- Approved by supermajority (2/3) of current Maintainers

### Project Lead

The Project Lead is responsible for overall project direction, conflict resolution, and representing the project externally (e.g., to the CNCF TOC). The initial Project Lead is the project founder.

## Decision Making

1. **Lazy consensus** — Most decisions are made through GitHub issues and pull requests. If no one objects within 7 days, the proposal is accepted.
2. **Vote** — For contentious decisions, a vote may be called. Each Maintainer gets one vote. Decisions require a simple majority unless otherwise specified.
3. **Project Lead tie-break** — If a vote is tied, the Project Lead casts the deciding vote.

## Changes to Governance

Changes to this governance model require a supermajority (2/3) vote of all Maintainers.

## Code of Conduct

All participants must follow the project's [Code of Conduct](CODE_OF_CONDUCT.md).

## CNCF Alignment

This project follows CNCF governance best practices and intends to donate to the CNCF as a Sandbox project. Upon acceptance, governance will be updated to align with CNCF requirements.
