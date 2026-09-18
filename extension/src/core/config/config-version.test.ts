// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

import { parseConfigVersion, parseSemVer, compareConfigVersions } from './config-version';

describe('config-version', () => {
	describe('parseConfigVersion', () => {
		it('should extract double-quoted version string', () => {
			const yaml = 'version: "1.1.0"\nport: 8000';
			expect(parseConfigVersion(yaml)).toBe('1.1.0');
		});

		it('should extract single-quoted version string', () => {
			const yaml = "version: '2.0.1'\nport: 8000";
			expect(parseConfigVersion(yaml)).toBe('2.0.1');
		});

		it('should extract unquoted version string', () => {
			const yaml = 'version: 1.2.0\nport: 8000';
			expect(parseConfigVersion(yaml)).toBe('1.2.0');
		});

		it('should strip trailing comments', () => {
			const yaml = 'version: "1.1.0" # Schema version\nport: 8000';
			expect(parseConfigVersion(yaml)).toBe('1.1.0');
		});

		it('should handle leading whitespace or banner comments before version', () => {
			const yaml = '# Banner header\n\nversion: "1.1.0"\nport: 8000';
			expect(parseConfigVersion(yaml)).toBe('1.1.0');
		});

		it('should default to 1.0.0 when version key is missing (legacy profiles)', () => {
			const yaml = '# Old profile without version\nport: 8000\nhost: "127.0.0.1"';
			expect(parseConfigVersion(yaml)).toBe('1.0.0');
		});

		it('should default to 1.0.0 when version key is empty', () => {
			const yaml = 'version:\nport: 8000';
			expect(parseConfigVersion(yaml)).toBe('1.0.0');
		});
	});

	describe('parseSemVer', () => {
		it('should parse valid semver string', () => {
			expect(parseSemVer('1.2.3')).toEqual({ major: 1, minor: 2, patch: 3 });
		});

		it('should handle leading v', () => {
			expect(parseSemVer('v2.0.5')).toEqual({ major: 2, minor: 0, patch: 5 });
		});

		it('should handle partial or missing version parts', () => {
			expect(parseSemVer('2.1')).toEqual({ major: 2, minor: 1, patch: 0 });
			expect(parseSemVer('3')).toEqual({ major: 3, minor: 0, patch: 0 });
			expect(parseSemVer('')).toEqual({ major: 0, minor: 0, patch: 0 });
		});

		it('should handle non-numeric inputs gracefully', () => {
			expect(parseSemVer('invalid.text')).toEqual({ major: 0, minor: 0, patch: 0 });
		});
	});

	describe('compareConfigVersions', () => {
		it('should return major when target major is greater', () => {
			expect(compareConfigVersions('1.0.0', '2.0.0')).toBe('major');
			expect(compareConfigVersions('1.9.9', '2.0.0')).toBe('major');
		});

		it('should return minor when target minor is greater with same major', () => {
			expect(compareConfigVersions('1.0.0', '1.1.0')).toBe('minor');
			expect(compareConfigVersions('1.0.5', '1.1.0')).toBe('minor');
		});

		it('should return patch when target patch is greater with same major and minor', () => {
			expect(compareConfigVersions('1.1.0', '1.1.1')).toBe('patch');
			expect(compareConfigVersions('1.1.2', '1.1.5')).toBe('patch');
		});

		it('should return equal when versions are identical', () => {
			expect(compareConfigVersions('1.1.0', '1.1.0')).toBe('equal');
			expect(compareConfigVersions('2.0.0', '2.0.0')).toBe('equal');
		});

		it('should return equal when current version is ahead of target', () => {
			expect(compareConfigVersions('2.0.0', '1.1.0')).toBe('equal');
			expect(compareConfigVersions('1.2.0', '1.1.0')).toBe('equal');
			expect(compareConfigVersions('1.1.2', '1.1.1')).toBe('equal');
		});
	});
});
