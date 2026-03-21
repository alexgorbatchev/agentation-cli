#!/usr/bin/env node

const { spawn } = require('node:child_process');
const { getBinaryPath } = require('../lib/getBinaryPath');

function run() {
  let binaryPath;

  try {
    binaryPath = getBinaryPath();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }

  const child = spawn(binaryPath, process.argv.slice(2), {
    stdio: 'inherit',
  });

  child.on('error', (error) => {
    console.error(`Failed to launch agentation: ${error.message}`);
    process.exit(1);
  });

  child.on('exit', (code, signal) => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }

    process.exit(code ?? 1);
  });
}

run();
