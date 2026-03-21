const BINARY_NAME = 'agentation';
const PACKAGE_SCOPE = '@alexgorbatchev';
const RELEASE_REPOSITORY = 'alexgorbatchev/agentation-cli';

const distributionTargets = [
  {
    key: 'darwin:x64',
    workspaceDirectory: 'npm/darwin-x64',
    packageName: `${PACKAGE_SCOPE}/agentation-cli-darwin-x64`,
    os: 'darwin',
    cpu: 'x64',
    goos: 'darwin',
    goarch: 'amd64',
  },
  {
    key: 'darwin:arm64',
    workspaceDirectory: 'npm/darwin-arm64',
    packageName: `${PACKAGE_SCOPE}/agentation-cli-darwin-arm64`,
    os: 'darwin',
    cpu: 'arm64',
    goos: 'darwin',
    goarch: 'arm64',
  },
  {
    key: 'linux:x64',
    workspaceDirectory: 'npm/linux-x64',
    packageName: `${PACKAGE_SCOPE}/agentation-cli-linux-x64`,
    os: 'linux',
    cpu: 'x64',
    goos: 'linux',
    goarch: 'amd64',
  },
  {
    key: 'linux:arm64',
    workspaceDirectory: 'npm/linux-arm64',
    packageName: `${PACKAGE_SCOPE}/agentation-cli-linux-arm64`,
    os: 'linux',
    cpu: 'arm64',
    goos: 'linux',
    goarch: 'arm64',
  },
];

function archiveNameForTarget(target, version) {
  return `agentation-cli_${version}_${target.goos}_${target.goarch}.tar.gz`;
}

function checksumsAssetName(version) {
  return `agentation-cli_${version}_checksums.txt`;
}

function getTargetForPlatform(os, cpu) {
  return distributionTargets.find((target) => target.os === os && target.cpu === cpu) ?? null;
}

function getTargetForCurrentPlatform() {
  return getTargetForPlatform(process.platform, process.arch);
}

module.exports = {
  BINARY_NAME,
  RELEASE_REPOSITORY,
  archiveNameForTarget,
  checksumsAssetName,
  distributionTargets,
  getTargetForCurrentPlatform,
  getTargetForPlatform,
};
