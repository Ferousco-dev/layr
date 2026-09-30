# Source control plan

- One repository; `main` is always releasable. Work happens on short-lived branches named `<type>/<topic>` (`feat/`, `fix/`, `docs/`).
- The maintainer makes all commits, pushes and tags. Assistants write code and never run git write commands.
- Commits are small and reference a `CR-###`, `REQ-###` or `DEF-###` in the message.
- Releases are tagged `vMAJOR.MINOR.PATCH` on `main` after CI is green and the gate record in `.ilana/gates/` is complete.
- Every change to scope goes through `.ilana/changes.md` before code.
