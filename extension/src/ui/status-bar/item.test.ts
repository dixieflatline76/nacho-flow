import * as vscode from 'vscode';
import { StatusBarManager } from './item';

// Mock VS Code API
jest.mock('vscode', () => {
  class MockMarkdownString {
    public value: string = '';
    public isTrusted: boolean = false;
    public supportThemeIcons: boolean = false;
    constructor(value?: string) {
      if (value) this.value = value;
    }
    appendMarkdown(val: string) {
      this.value += val;
    }
  }

  return {
    window: {
      createStatusBarItem: jest.fn().mockReturnValue({
        show: jest.fn(),
        hide: jest.fn(),
        dispose: jest.fn(),
        text: '',
        tooltip: ''
      })
    },
    StatusBarAlignment: {
      Right: 1
    },
    MarkdownString: MockMarkdownString,
    workspace: {
      getConfiguration: jest.fn().mockReturnValue({
        get: jest.fn()
      })
    }
  };
}, { virtual: true });

describe('StatusBarManager', () => {
  let statusBarManager: StatusBarManager;
  let mockStatusBarItem: any;

  beforeEach(() => {
    // Reset mocks
    jest.clearAllMocks();

    // Create mock status bar item
    mockStatusBarItem = {
      show: jest.fn(),
      hide: jest.fn(),
      dispose: jest.fn(),
      text: '',
      tooltip: ''
    };

    // Mock createStatusBarItem to return our mock
    (vscode.window.createStatusBarItem as jest.Mock).mockReturnValue(mockStatusBarItem);

    // Create StatusBarManager instance
    statusBarManager = new StatusBarManager();
  });

  describe('constructor', () => {
    it('should create a status bar item with correct properties', () => {
      expect(vscode.window.createStatusBarItem).toHaveBeenCalledWith(vscode.StatusBarAlignment.Right, 100);
      expect(mockStatusBarItem.command).toBe('nacho-flow.showDashboard');
      expect(mockStatusBarItem.show).toHaveBeenCalled();
    });
  });

  describe('updateStats', () => {
    it('should update status bar with stats data', () => {
      const stats = {
        estimated_cost_saved_usd: 10.5,
        total_requests: 100,
        tier_breakdown: {
          tier1_local_free: 75
        }
      };

      statusBarManager.updateStats(stats);

      expect(mockStatusBarItem.text).toBe('🌮 $10.50 Saved (75% Local)');
      expect(mockStatusBarItem.tooltip.value).toContain('Est. Cost Saved');
      expect(mockStatusBarItem.tooltip.value).toContain('Local GPU');
      expect(mockStatusBarItem.show).toHaveBeenCalled();
    });

    it('should handle null stats', () => {
      statusBarManager.updateStats(null);

      expect(mockStatusBarItem.text).toBe('$(circle-slash) Nacho: Offline');
      expect(mockStatusBarItem.tooltip.value).toContain('Offline');
      expect(mockStatusBarItem.show).toHaveBeenCalled();
    });
  });

  describe('calculateLocalPercentage', () => {
    it('should calculate local percentage correctly', () => {
      const stats = {
        total_requests: 200,
        tier_breakdown: {
          tier1_local_free: 50
        }
      };

      statusBarManager.updateStats(stats);

      // Access private method through casting
      const localPercentage = (statusBarManager as any).calculateLocalPercentage();
      expect(localPercentage).toBe(25);
    });

    it('should calculate local percentage based on tokens when available', () => {
      const stats = {
        total_tokens: 100000,
        total_tokens_routed_locally: 80000
      };

      statusBarManager.updateStats(stats);
      const localPercentage = (statusBarManager as any).calculateLocalPercentage();
      expect(localPercentage).toBe(80);
    });

    it('should return 0 when total requests is 0', () => {
      const stats = {
        total_requests: 0,
        tier_breakdown: {
          tier1_local_free: 0
        }
      };

      statusBarManager.updateStats(stats);

      const localPercentage = (statusBarManager as any).calculateLocalPercentage();
      expect(localPercentage).toBe(0);
    });
  });

  describe('setTimeWindow', () => {
    it('should update status bar text and tooltip when switching to today', () => {
      const stats = {
        windows: {
          today: {
            cost_saved_usd: 2.5,
            cost_spent_usd: 0.5,
            requests: 20,
            tokens_total: 10000,
            tokens_local: 8000,
            cost_reduction_pct: 83
          }
        }
      };

      statusBarManager.updateStats(stats);
      statusBarManager.setTimeWindow('today');

      expect(statusBarManager.getTimeWindow()).toBe('today');
      expect(mockStatusBarItem.text).toBe('🌮 $2.50 Saved Today (80% Local)');
      expect(mockStatusBarItem.tooltip.value).toContain('Today (Active 24h)');
    });

    it('should update status bar text when switching to this_week and this_month', () => {
      const stats = {
        windows: {
          this_week: {
            cost_saved_usd: 12.0,
            requests: 50,
            tokens_total: 20000,
            tokens_local: 15000
          },
          this_month: {
            cost_saved_usd: 45.0,
            requests: 200,
            tokens_total: 100000,
            tokens_local: 90000
          }
        }
      };

      statusBarManager.updateStats(stats);

      statusBarManager.setTimeWindow('this_week');
      expect(mockStatusBarItem.text).toBe('🌮 $12.00 Saved This Week (75% Local)');

      statusBarManager.setTimeWindow('this_month');
      expect(mockStatusBarItem.text).toBe('🌮 $45.00 Saved This Month (90% Local)');
    });
  });

  describe('dispose', () => {
    it('should dispose the status bar item', () => {
      statusBarManager.dispose();
      expect(mockStatusBarItem.dispose).toHaveBeenCalled();
    });
  });
});