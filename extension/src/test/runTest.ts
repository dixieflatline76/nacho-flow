import * as path from 'path';
import * as fs from 'fs';

// Simple test to verify project structure
console.log('Verifying Nacho Flow extension project structure...');

const requiredFiles = [
	'package.json',
	'tsconfig.json',
	'src/extension.ts',
	'src/core/controller.ts',
	'src/core/api/client.ts',
	'src/core/config/auth-manager.ts',
	'src/core/sse/client.ts',
	'src/ui/status-bar/item.ts'
];

const basePath = path.join(__dirname, '..');

let allExist = true;
for (const file of requiredFiles) {
	const fullPath = path.join(basePath, file);
	if (!fs.existsSync(fullPath)) {
		console.error(`❌ Missing required file: ${file}`);
		allExist = false;
	} else {
		console.log(`✅ Found: ${file}`);
	}
}

if (allExist) {
	console.log('\n🎉 All required files are present!');
	console.log('Project structure is ready for development.');
} else {
	console.log('\n❌ Some required files are missing.');
	console.log('Please check the project structure and add missing files.');
}

export {};