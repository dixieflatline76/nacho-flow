import {
  formatCurrency,
  formatTokenCount,
  formatLatency,
  formatDate,
  formatTime,
  calculateSavingsPercentage
} from './formatters';

describe('Formatters', () => {
  describe('formatCurrency', () => {
    it('should format currency with two decimal places', () => {
      expect(formatCurrency(10)).toBe('$10.00');
      expect(formatCurrency(10.5)).toBe('$10.50');
      expect(formatCurrency(10.567)).toBe('$10.57');
    });
  });

  describe('formatTokenCount', () => {
    it('should format token counts with appropriate units', () => {
      expect(formatTokenCount(500)).toBe('500');
      expect(formatTokenCount(1500)).toBe('1.5K');
      expect(formatTokenCount(1500000)).toBe('1.5M');
    });
  });

  describe('formatLatency', () => {
    it('should format latency with appropriate units', () => {
      expect(formatLatency(500)).toBe('500.0ms');
      expect(formatLatency(1500)).toBe('1.5s');
    });
  });

  describe('formatDate', () => {
    it('should format ISO date string to locale date string', () => {
      const isoString = '2023-01-01T12:00:00.000Z';
      const result = formatDate(isoString);
      expect(result).toBe(new Date(isoString).toLocaleString());
    });
  });

  describe('formatTime', () => {
    it('should format ISO date string to locale time string', () => {
      const isoString = '2023-01-01T12:00:00.000Z';
      const result = formatTime(isoString);
      expect(result).toBe(new Date(isoString).toLocaleTimeString());
    });
  });

  describe('calculateSavingsPercentage', () => {
    it('should calculate savings percentage correctly', () => {
      expect(calculateSavingsPercentage(100, 50)).toBe(-50);
      expect(calculateSavingsPercentage(100, 150)).toBe(50);
      expect(calculateSavingsPercentage(0, 50)).toBe(0);
    });
  });
});