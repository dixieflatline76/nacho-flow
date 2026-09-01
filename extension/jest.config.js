// Shared options inherited by both test projects.
const sharedOptions = {
  preset: 'ts-jest',
  collectCoverageFrom: [
    'src/**/*.{ts,tsx}',
    '!src/**/*.d.ts',
    '!src/**/*.test.ts', // test files are not source under test
    '!src/test/**',
    '!src/extension.ts', // entry point only
  ],
  coverageReporters: ['text', 'lcov', 'html'],
  coverageThreshold: {
    global: {
      statements: 95,
      branches: 80,
      functions: 95,
      lines: 95,
    },
  },
  moduleDirectories: ['node_modules', 'src'],
  setupFilesAfterEnv: ['<rootDir>/src/test/setup.ts'],
};

module.exports = {
  collectCoverage: true,
  coverageReporters: sharedOptions.coverageReporters,
  coverageThreshold: sharedOptions.coverageThreshold,
  collectCoverageFrom: sharedOptions.collectCoverageFrom,
  projects: [
    // ── Node environment — all tests except webview JS tests ──────────────
    {
      ...sharedOptions,
      displayName: 'node',
      testEnvironment: 'node',
      testMatch: [
        '**/*.test.ts',
        '!**/dashboard-webview.test.ts', // handled by jsdom project below
      ],
    },
    // ── JSDOM environment — plain-JS webview runtime tests ────────────────
    {
      ...sharedOptions,
      displayName: 'jsdom',
      testEnvironment: 'jest-environment-jsdom',
      testMatch: ['**/dashboard-webview.test.ts'],
    },
  ],
};
