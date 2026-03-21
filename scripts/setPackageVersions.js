#!/usr/bin/env node

const fs = require('node:fs');
const path = require('node:path');

const rootDirectoryPath = path.resolve(__dirname, '..');
const rootPackageJsonPath = path.join(rootDirectoryPath, 'package.json');
const distributionPackageJsonPaths = [
  path.join(rootDirectoryPath, 'npm', 'darwin-x64', 'package.json'),
  path.join(rootDirectoryPath, 'npm', 'darwin-arm64', 'package.json'),
  path.join(rootDirectoryPath, 'npm', 'linux-x64', 'package.json'),
  path.join(rootDirectoryPath, 'npm', 'linux-arm64', 'package.json'),
];

function writePackageJson(packageJsonPath, packageJson) {
  fs.writeFileSync(packageJsonPath, `${JSON.stringify(packageJson, null, 2)}\n`);
}

function updateRootPackage(rootPackageJson, version) {
  rootPackageJson.version = version;

  for (const dependencyName of Object.keys(rootPackageJson.optionalDependencies ?? {})) {
    rootPackageJson.optionalDependencies[dependencyName] = version;
  }

  return rootPackageJson;
}

function main() {
  const version = process.argv[2];
  if (!version) {
    throw new Error('Usage: node ./scripts/setPackageVersions.js <version>');
  }

  const rootPackageJson = JSON.parse(fs.readFileSync(rootPackageJsonPath, 'utf8'));
  writePackageJson(rootPackageJsonPath, updateRootPackage(rootPackageJson, version));

  for (const packageJsonPath of distributionPackageJsonPaths) {
    const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
    packageJson.version = version;
    writePackageJson(packageJsonPath, packageJson);
  }
}

try {
  main();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
