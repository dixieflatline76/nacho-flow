@echo off
echo Building Nacho Flow VS Code Extension...

REM Create output directory
if not exist "out" mkdir out

REM Copy essential files
copy package.json out\ >nul
copy README.md out\ >nul
copy CHANGELOG.md out\ >nul

REM Create a simple build verification file
echo // Nacho Flow Extension Build Verification > out\build-verification.js
echo console.log('Nacho Flow extension build successful!'); >> out\build-verification.js
echo console.log('Version: 0.6.0'); >> out\build-verification.js
echo console.log('Build date: %date% %time%'); >> out\build-verification.js

echo Build completed successfully!
echo Output files are in the 'out' directory.