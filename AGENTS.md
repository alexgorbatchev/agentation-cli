# Agentation CLI Release Instructions

## Scope

This repository publishes:

1. wrapper package: `@alexgorbatchev/agentation-cli`
2. platform package: `@alexgorbatchev/agentation-cli-darwin-x64`
3. platform package: `@alexgorbatchev/agentation-cli-darwin-arm64`
4. platform package: `@alexgorbatchev/agentation-cli-linux-x64`
5. platform package: `@alexgorbatchev/agentation-cli-linux-arm64`

It also publishes GitHub release archives via GoReleaser.

## Trusted publishing requirements

Before releasing, all five npm packages must already exist on npm and each one must be configured with the same trusted publisher:

- GitHub user/org: `alexgorbatchev`
- repository: `agentation-cli`
- workflow filename: `release.yml`

The workflow file must remain at `.github/workflows/release.yml`. If the filename changes, trusted publishing breaks until npm settings are updated for all five packages.

## Correct release flow

### 1. Bump package versions in the repo

Run:

```bash
node ./scripts/setPackageVersions.js <version>
npm install --package-lock-only --ignore-scripts
```

Commit the changed files before tagging.

Files expected to change:

- `package.json`
- `package-lock.json`
- `npm/darwin-x64/package.json`
- `npm/darwin-arm64/package.json`
- `npm/linux-x64/package.json`
- `npm/linux-arm64/package.json`

### 2. Push commits first

Push `main` before creating the tag.

```bash
git push origin main
```

### 3. Create and push the release tag

```bash
git tag -a v<version> -m "v<version>"
git push origin v<version>
```

Pushing the tag triggers `.github/workflows/release.yml`.

## What the release workflow does

The release workflow:

1. checks out the repo
2. installs Go and Node
3. runs GoReleaser to build and publish GitHub release assets
4. syncs npm package versions from the tag with `scripts/setPackageVersions.js`
5. installs npm dependencies with scripts disabled
6. extracts binaries into each platform package with `scripts/stageNpmPackages.js`
7. publishes the four platform npm packages with trusted publishing
8. publishes the wrapper package last

## Why platform packages publish first

The wrapper package depends on platform packages through `optionalDependencies`. If the wrapper publishes first, installs can fail before the platform packages are available.

## Post-release verification

After GitHub Actions finishes successfully, verify both install paths.

### Normal install path

```bash
mkdir -p /tmp/agentation-cli-test-normal
cd /tmp/agentation-cli-test-normal
npm init -y
npm install @alexgorbatchev/agentation-cli
./node_modules/.bin/agentation version
```

### Fallback download path

```bash
mkdir -p /tmp/agentation-cli-test-fallback
cd /tmp/agentation-cli-test-fallback
npm init -y
npm install --omit=optional @alexgorbatchev/agentation-cli
./node_modules/.bin/agentation version
```

The fallback path validates the GitHub-release download logic in `scripts/install.js`.

## Do not do these things

- Do not publish the wrapper package before the platform packages.
- Do not change archive naming in GoReleaser without updating `lib/platform.js`.
- Do not rename `release.yml` without updating trusted publisher config on npm for all five packages.
- Do not assume `--omit=optional` works unless you verify it after release.

## Bootstrap note

`scripts/publishBootstrapPackages.sh` is only a convenience helper for opening the npm package pages. It is not part of the real release flow.
