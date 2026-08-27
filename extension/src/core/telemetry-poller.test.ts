import { TelemetryPoller } from './telemetry-poller';

describe('TelemetryPoller', () => {
	beforeEach(() => {
		jest.useFakeTimers();
	});

	afterEach(() => {
		jest.useRealTimers();
		jest.restoreAllMocks();
	});

	it('should default to 60 seconds interval and arm timer', async () => {
		const onTick = jest.fn().mockResolvedValue(undefined);
		const poller = new TelemetryPoller({ onTick });

		expect(poller.getIntervalSeconds()).toBe(60);
		expect(poller.isRunning()).toBe(true);

		jest.advanceTimersByTime(59000);
		expect(onTick).not.toHaveBeenCalled();

		jest.advanceTimersByTime(1000);
		expect(onTick).toHaveBeenCalledTimes(1);

		poller.dispose();
	});

	it('should accept custom intervals like 30s or 15s', async () => {
		const onTick = jest.fn().mockResolvedValue(undefined);
		const poller = new TelemetryPoller({ intervalSeconds: 15, onTick });

		expect(poller.getIntervalSeconds()).toBe(15);
		expect(poller.isRunning()).toBe(true);

		jest.advanceTimersByTime(15000);
		await Promise.resolve(); // let microtasks settle
		expect(onTick).toHaveBeenCalledTimes(1);

		poller.setIntervalSeconds(30);
		expect(poller.getIntervalSeconds()).toBe(30);

		jest.advanceTimersByTime(15000);
		await Promise.resolve();
		expect(onTick).toHaveBeenCalledTimes(1); // not 30s yet

		jest.advanceTimersByTime(15000);
		await Promise.resolve();
		expect(onTick).toHaveBeenCalledTimes(2);

		poller.dispose();
	});

	it('should disable timer when interval is set to 0 (Off)', async () => {
		const onTick = jest.fn().mockResolvedValue(undefined);
		const poller = new TelemetryPoller({ intervalSeconds: 0, onTick });

		expect(poller.getIntervalSeconds()).toBe(0);
		expect(poller.isRunning()).toBe(false);

		jest.advanceTimersByTime(120000);
		expect(onTick).not.toHaveBeenCalled();

		// Re-enable to 60s
		poller.setIntervalSeconds(60);
		expect(poller.isRunning()).toBe(true);

		jest.advanceTimersByTime(60000);
		await Promise.resolve();
		expect(onTick).toHaveBeenCalledTimes(1);

		poller.dispose();
	});

	it('should pause and resume cleanly with or without immediate tick', async () => {
		const onTick = jest.fn().mockResolvedValue(undefined);
		const poller = new TelemetryPoller({ intervalSeconds: 30, onTick });

		poller.pause();
		expect(poller.isRunning()).toBe(false);

		// Multiple pause calls should be idempotent
		poller.pause();
		expect(poller.isRunning()).toBe(false);

		jest.advanceTimersByTime(60000);
		expect(onTick).not.toHaveBeenCalled();

		// Resume without immediate tick
		poller.resume(false);
		expect(poller.isRunning()).toBe(true);
		expect(onTick).not.toHaveBeenCalled();

		jest.advanceTimersByTime(30000);
		await Promise.resolve();
		expect(onTick).toHaveBeenCalledTimes(1);

		// Pause and resume with immediate tick
		poller.pause();
		poller.resume(true);
		await Promise.resolve();
		expect(onTick).toHaveBeenCalledTimes(2);

		// Multiple resume calls should be idempotent
		poller.resume(true);
		expect(onTick).toHaveBeenCalledTimes(2);

		poller.dispose();
	});

	it('should handle setIntervalSeconds when paused or with 0 seconds', () => {
		const onTick = jest.fn().mockResolvedValue(undefined);
		const poller = new TelemetryPoller({ intervalSeconds: 30, onTick });

		poller.pause();
		poller.setIntervalSeconds(15);
		expect(poller.getIntervalSeconds()).toBe(15);
		expect(poller.isRunning()).toBe(false);

		poller.resume(false);
		expect(poller.isRunning()).toBe(true);

		poller.setIntervalSeconds(0);
		expect(poller.isRunning()).toBe(false);

		poller.dispose();
	});

	it('should enforce Single-Flight concurrency lock preventing overlapping ticks', async () => {
		let resolveTick!: () => void;
		const slowPromise = new Promise<void>((resolve) => {
			resolveTick = resolve;
		});

		const onTick = jest.fn().mockImplementation(() => slowPromise);
		const poller = new TelemetryPoller({ intervalSeconds: 15, onTick });

		// Trigger tick 1
		const tickPromise1 = poller.executeTick();
		expect(onTick).toHaveBeenCalledTimes(1);

		// Trigger tick 2 while tick 1 is still in-flight
		const tickPromise2 = poller.executeTick();
		expect(onTick).toHaveBeenCalledTimes(1); // Blocked by isExecuting

		resolveTick();
		await tickPromise1;
		await tickPromise2;

		// Next tick should now execute
		await poller.executeTick();
		expect(onTick).toHaveBeenCalledTimes(2);

		poller.dispose();
	});

	it('should handle tick errors gracefully without breaking the timer loop', async () => {
		const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
		const onTick = jest.fn().mockRejectedValue(new Error('Network offline'));
		const poller = new TelemetryPoller({ intervalSeconds: 15, onTick });

		await poller.executeTick();
		expect(onTick).toHaveBeenCalledTimes(1);
		expect(consoleSpy).toHaveBeenCalled();

		// Ensure next timer tick still works
		onTick.mockResolvedValueOnce(undefined);
		jest.advanceTimersByTime(15000);
		await Promise.resolve();
		expect(onTick).toHaveBeenCalledTimes(2);

		poller.dispose();
	});

	it('should stop and prevent further executions when disposed', async () => {
		const onTick = jest.fn().mockResolvedValue(undefined);
		const poller = new TelemetryPoller({ intervalSeconds: 15, onTick });

		poller.dispose();
		expect(poller.isRunning()).toBe(false);

		// Multiple dispose calls should be safe
		poller.dispose();

		await poller.executeTick();
		expect(onTick).not.toHaveBeenCalled();

		poller.setIntervalSeconds(30);
		expect(poller.isRunning()).toBe(false);

		poller.pause();
		expect(poller.isRunning()).toBe(false);

		poller.resume();
		expect(poller.isRunning()).toBe(false);
	});
});
