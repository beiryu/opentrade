# Security policy

## Threat model

OpenTrade is a local tool. It binds to `127.0.0.1`, has no user accounts and no authentication. Its one secret is the model API key read from the environment or a local `.env`. It writes a SQLite file under `data/local/`. Today it places no real orders anywhere.

If you expose the port beyond your machine, anyone who can reach it can read every prompt and run cycles. Do not do that; put it behind a reverse proxy with authentication if you must.

## Rules the project keeps

- No key, token, database or decision log is ever committed. `.env`, `*.db` and `data/local/` are gitignored; CI greps for leaks.
- Orders can only reach a broker (paper today, real later) through `engine.ApplyGuards` and `paper.Account.Submit`. A change that bypasses them is a vulnerability, not a feature.
- Dependencies are few and pinned; `govulncheck` runs in CI; Dependabot opens updates.
- A future real-order adapter will be opt-in, off by default, and will never store broker credentials in the repository or the database in clear text.

## Reporting a vulnerability

Please report privately through [GitHub security advisories](https://github.com/beiryu/opentrade/security/advisories/new). Include steps to reproduce and the version (`opentrade version`). You will get an acknowledgement within a week; fixes for confirmed issues are released as soon as they are ready and credited in the changelog unless you prefer otherwise.

Do not open a public issue for a security problem.
