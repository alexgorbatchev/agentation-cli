const fs = require('node:fs');
const path = require('node:path');
const { BINARY_NAME, getTargetForCurrentPlatform } = require('./platform');

const fallbackBinaryPath = path.join(__dirname, '..', 'downloaded', BINARY_NAME);

function resolveOptionalDependencyBinaryPath() {
  const target = getTargetForCurrentPlatform();
  if (target === null) {
    return null;
  }

  try {
    return require.resolve(`${target.packageName}/bin/${BINARY_NAME}`);
  } catch (_error) {
    return null;
  }
}

function getBinaryPath() {
  const optionalDependencyBinaryPath = resolveOptionalDependencyBinaryPath();
  if (optionalDependencyBinaryPath !== null) {
    return optionalDependencyBinaryPath;
  }

  if (fs.existsSync(fallbackBinaryPath)) {
    return fallbackBinaryPath;
  }

  const target = getTargetForCurrentPlatform();
  if (target === null) {
    throw new Error(
      `Unsupported platform ${process.platform}-${process.arch}. Supported targets: darwin-x64, darwin-arm64, linux-x64, linux-arm64.`
    );
  }

  throw new Error(
    [
      `The ${target.packageName} package was not installed and no fallback binary was downloaded.`,
      'Reinstall without --omit=optional (or --ignore-optional), or set AGENTATION_CLI_BINARY_HOST to a reachable mirror and reinstall.',
    ].join('\n')
  );
}

module.exports = {
  getBinaryPath,
  resolveOptionalDependencyBinaryPath,
};
