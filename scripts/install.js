#!/usr/bin/env node

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const tar = require('tar');
const packageJson = require('../package.json');
const { resolveOptionalDependencyBinaryPath } = require('../lib/getBinaryPath');
const {
  BINARY_NAME,
  RELEASE_REPOSITORY,
  archiveNameForTarget,
  checksumsAssetName,
  getTargetForCurrentPlatform,
} = require('../lib/platform');

const version = packageJson.version;
const rootDirectoryPath = path.resolve(__dirname, '..');
const downloadDirectoryPath = path.join(rootDirectoryPath, 'downloaded');
const temporaryDirectoryPath = path.join(rootDirectoryPath, '.agentation-cli-tmp');
const fallbackBinaryPath = path.join(downloadDirectoryPath, BINARY_NAME);

function getReleaseBaseUrl() {
  if (process.env.AGENTATION_CLI_BINARY_HOST) {
    return process.env.AGENTATION_CLI_BINARY_HOST.replace(/\/$/, '');
  }

  return `https://github.com/${RELEASE_REPOSITORY}/releases/download/v${version}`;
}

async function fetchResponse(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Request failed for ${url}: ${response.status} ${response.statusText}`);
  }
  return response;
}

async function downloadText(url) {
  const response = await fetchResponse(url);
  return response.text();
}

async function downloadFile(url, filePath) {
  const response = await fetchResponse(url);
  const arrayBuffer = await response.arrayBuffer();
  fs.writeFileSync(filePath, Buffer.from(arrayBuffer));
}

function parseChecksums(checksumFileContent) {
  const checksumsByFileName = new Map();

  for (const line of checksumFileContent.split(/\r?\n/)) {
    const trimmedLine = line.trim();
    if (trimmedLine.length === 0) {
      continue;
    }

    const [checksum, fileName] = trimmedLine.split(/\s+/);
    if (checksum && fileName) {
      checksumsByFileName.set(fileName, checksum);
    }
  }

  return checksumsByFileName;
}

function validateChecksum(filePath, expectedChecksum) {
  const actualChecksum = crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex');
  if (actualChecksum !== expectedChecksum) {
    throw new Error(`Checksum mismatch for ${path.basename(filePath)}. Expected ${expectedChecksum}, received ${actualChecksum}.`);
  }
}

async function extractFallbackBinary(archivePath) {
  fs.rmSync(downloadDirectoryPath, { recursive: true, force: true });
  fs.mkdirSync(downloadDirectoryPath, { recursive: true });

  await tar.x({
    cwd: downloadDirectoryPath,
    file: archivePath,
    gzip: true,
    filter: (entryPath) => path.basename(entryPath) === BINARY_NAME,
  });

  if (!fs.existsSync(fallbackBinaryPath)) {
    throw new Error(`Archive ${path.basename(archivePath)} did not contain ${BINARY_NAME}.`);
  }

  fs.chmodSync(fallbackBinaryPath, 0o755);
}

async function downloadFallbackBinary() {
  const target = getTargetForCurrentPlatform();
  if (target === null) {
    throw new Error(
      `Unsupported platform ${process.platform}-${process.arch}. Supported targets: darwin-x64, darwin-arm64, linux-x64, linux-arm64.`
    );
  }

  const releaseBaseUrl = getReleaseBaseUrl();
  const archiveFileName = archiveNameForTarget(target, version);
  const archiveUrl = `${releaseBaseUrl}/${archiveFileName}`;
  const checksumUrl = `${releaseBaseUrl}/${checksumsAssetName(version)}`;
  const archivePath = path.join(temporaryDirectoryPath, `${archiveFileName}.tmp`);

  fs.mkdirSync(downloadDirectoryPath, { recursive: true });
  fs.mkdirSync(temporaryDirectoryPath, { recursive: true });

  console.error(`Downloading fallback binary from ${archiveUrl}`);

  const checksumFileContent = await downloadText(checksumUrl);
  const checksumsByFileName = parseChecksums(checksumFileContent);
  const expectedChecksum = checksumsByFileName.get(archiveFileName);
  if (!expectedChecksum) {
    throw new Error(`Could not find a checksum entry for ${archiveFileName} in ${checksumUrl}.`);
  }

  await downloadFile(archiveUrl, archivePath);
  validateChecksum(archivePath, expectedChecksum);
  await extractFallbackBinary(archivePath);
  fs.rmSync(archivePath, { force: true });
  fs.rmSync(temporaryDirectoryPath, { recursive: true, force: true });
}

async function main() {
  if (process.env.AGENTATION_CLI_SKIP_DOWNLOAD === '1') {
    return;
  }

  if (resolveOptionalDependencyBinaryPath() !== null) {
    return;
  }

  await downloadFallbackBinary();
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
