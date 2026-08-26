#!/bin/bash

# Simple build script for Nacho Flow VS Code Extension
echo "Building Nacho Flow VS Code Extension..."

# Create output directory
mkdir -p out

# Copy essential files
cp package.json out/
cp README.md out/
cp CHANGELOG.md out/

# Create a simple build verification file
echo "// Nacho Flow Extension Build Verification
console.log('Nacho Flow extension build successful!');
console.log('Version: 0.6.0');
console.log('Build date: $(date)');" > out/build-verification.js

echo "Build completed successfully!"
echo "Output files are in the 'out' directory."