/**
 * Utility functions for formatting data in the Nacho Flow extension
 */

export function formatCurrency(amount: number): string {
	return `$${amount.toFixed(2)}`;
}

export function formatTokenCount(tokens: number): string {
	if (tokens >= 1000000) {
		return `${(tokens / 1000000).toFixed(1)}M`;
	} else if (tokens >= 1000) {
		return `${(tokens / 1000).toFixed(1)}K`;
	}
	return tokens.toString();
}

export function formatLatency(ms: number): string {
	if (ms >= 1000) {
		return `${(ms / 1000).toFixed(1)}s`;
	}
	return `${ms.toFixed(1)}ms`;
}

export function formatDate(isoString: string): string {
	const date = new Date(isoString);
	return date.toLocaleString();
}

export function formatTime(isoString: string): string {
	const date = new Date(isoString);
	return date.toLocaleTimeString();
}

export function calculateSavingsPercentage(current: number, projected: number): number {
	if (current === 0) return 0;
	return ((projected - current) / current) * 100;
}