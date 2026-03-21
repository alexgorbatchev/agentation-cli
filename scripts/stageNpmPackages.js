#!/usr/bin/env node

const fs = require('node:fs');
const path = require('node:path');
const tar = require('tar');
const packageJson = require('../package.json');
const { BINARY_NAME, archiveNameForTarget, distributionTargets } = require('../lib/platform');

const rootDirectoryPath = path.resolve(__dirname, '..');
const distDirectoryPath = path.join(rootDirectoryPath, 'dist');

async function stageDistributionPackage(target) {
  const archivePath = path.join(distDirectoryPath, archiveNameForTarget(target, packageJson.version));
  if (!fs.existsSync(archivePath)) {
    throw new Error(`Missing archive ${archivePath}. Build release artifacts before staging npm packages.`);
  }

  const packageDirectoryPath = path.join(rootDirectoryPath, target.workspaceDirectory);
  const binDirectoryPath = path.join(packageDirectoryPath, 'bin');
  const binaryPath = path.join(binDirectoryPath, BINARY_NAME);

  fs.rmSync(binDirectoryPath, { recursive: true, force: true });
  fs.mkdirSync(binDirectoryPath, { recursive: true });

  await tar.x({
    cwd: binDirectoryPath,
    file: archivePath,
    gzip: true,
    filter: (entryPath) => path.basename(entryPath) === BINARY_NAME,
  });

  if (!fs.existsSync(binaryPath)) {
    throw new Error(`Archive ${path.basename(archivePath)} did not contain ${BINARY_NAME}.`);
  }

  fs.chmodSync(binaryPath, 0o755);
  console.log(`Staged ${target.packageName} from ${path.basename(archivePath)}`);
}

async function main() {
  for (const target of distributionTargets) {
    await stageDistributionPackage(target);
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
