@echo off
echo Packaging Nacho Flow VS Code Extension...

REM Create a simple package structure
if not exist "dist" mkdir dist

REM Copy all source files to dist
xcopy src dist\src\ /E /I /Y >nul
xcopy resources dist\resources\ /E /I /Y >nul

REM Copy configuration files
copy package.json dist\ >nul
copy README.md dist\ >nul
copy CHANGELOG.md dist\ >nul
copy tsconfig.json dist\ >nul

echo Extension packaged successfully!
echo Package files are in the 'dist' directory.
echo.
echo To install in VS Code:
echo 1. Open VS Code
echo 2. Go to Extensions (Ctrl+Shift+X)
echo 3. Click "..." menu and select "Install from VSIX"
echo 4. Select the packaged extension file