# Contributing

Thanks for helping improve `agentctl`.

## Branch flow

This repository uses a lightweight two-branch flow:

- `dev` is for day-to-day development, experiments, and contributor pull requests.
- `main` is the stable release branch.
- Release tags, such as `v0.2.0`, should be created from `main` only.

For normal changes, branch from `dev`:

```bash
git switch dev
git pull
git switch -c feature/my-change
```

Open pull requests back into `dev`. When `dev` is ready to release, merge it into `main`, create a version tag, and push the tag:

```bash
git switch main
git pull
git merge --ff-only dev
git tag -a vX.Y.Z -m "agentctl vX.Y.Z"
git push origin main vX.Y.Z
```

The release workflow runs for pushed tags that match `v*`.

## Checks

Before opening a pull request, run:

```bash
go test ./...
```

If you change command behavior, update `README.md` and `skills/agentctl/SKILL.md` when the agent-facing workflow changes too.
