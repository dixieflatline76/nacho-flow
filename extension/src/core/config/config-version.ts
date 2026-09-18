// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

export type VersionDelta = 'major' | 'minor' | 'patch' | 'equal';

export interface SemVer {
	major: number;
	minor: number;
	patch: number;
}

/**
 * Extracts the `version:` field from a YAML configuration document.
 * If the field is missing or empty, defaults to '1.0.0' for legacy unversioned profiles.
 */
export function parseConfigVersion(yamlContent: string): string {
	const match = /^[ \t]*version:[ \t]*["']?([^"'\r\n#]+)/m.exec(yamlContent);
	if (!match || !match[1].trim()) {
		return '1.0.0';
	}
	return match[1].trim();
}

/**
 * Parses a semantic version string (e.g. "1.1.0", "v2.0.0") into numeric components.
 */
export function parseSemVer(versionStr: string): SemVer {
	const clean = versionStr.trim().replace(/^v/i, '');
	const parts = clean.split('.').map(part => {
		const parsed = parseInt(part, 10);
		return isNaN(parsed) ? 0 : parsed;
	});

	return {
		major: parts[0] ?? 0,
		minor: parts[1] ?? 0,
		patch: parts[2] ?? 0
	};
}

/**
 * Compares the user profile version against the target factory template version.
 * Returns the classification of the update:
 * - 'major': Target has a higher major version (breaking change, reset required)
 * - 'minor': Target has a higher minor version (new additive features, diff/reset offered)
 * - 'patch': Target has a higher patch version (minor doc/default tweaks, seamless)
 * - 'equal': User profile is at or ahead of the template version
 */
export function compareConfigVersions(currentVersion: string, targetVersion: string): VersionDelta {
	const current = parseSemVer(currentVersion);
	const target = parseSemVer(targetVersion);

	if (target.major > current.major) {
		return 'major';
	}
	if (target.major === current.major && target.minor > current.minor) {
		return 'minor';
	}
	if (target.major === current.major && target.minor === current.minor && target.patch > current.patch) {
		return 'patch';
	}
	return 'equal';
}
