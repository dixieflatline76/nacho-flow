// Nacho Flow Activity Bar Sidebar Controller
(function() {
	// @ts-ignore
	const vscode = acquireVsCodeApi();
	let state = {
		engineMode: 'local', // 'local' | 'remote'
		engineStatus: { connected: false, version: '', error: '' },
		ollamaStatus: { connected: false, models: [] },
		hasOpenRouterKey: false,
		remoteUrl: 'http://127.0.0.1:8000',
		hasToken: false
	};

	// DOM Elements
	const engineStatusChip = document.getElementById('engine-status-chip');
	const radioLocal = document.getElementById('radio-mode-local');
	const radioRemote = document.getElementById('radio-mode-remote');
	const localControls = document.getElementById('local-engine-controls');
	const remoteControls = document.getElementById('remote-engine-controls');
	const btnEngineStart = document.getElementById('btn-engine-start');
	const btnEngineStop = document.getElementById('btn-engine-stop');
	const remoteUrlInput = document.getElementById('remote-engine-url');
	const remoteTokenInput = document.getElementById('remote-engine-token');
	const ollamaStatusChip = document.getElementById('ollama-status-chip');
	const ollamaModelText = document.getElementById('ollama-model-text');
	const openrouterStatusChip = document.getElementById('openrouter-status-chip');
	const openrouterDetailText = document.getElementById('openrouter-detail-text');
	const proxyEndpointText = document.getElementById('proxy-endpoint-text');
	const specsModal = document.getElementById('specs-modal');
	const dangerModal = document.getElementById('danger-modal');

	// Handle messages from VS Code extension
	window.addEventListener('message', event => {
		const message = event.data;
		switch (message.command) {
			case 'updateState':
				applyState(message.data);
				break;
			case 'updateEngineStatus':
				updateEngineStatus(message.data);
				break;
			case 'updateOllamaStatus':
				updateOllamaStatus(message.data);
				break;
		}
	});

	function applyState(newState) {
		state = { ...state, ...newState };
		
		// Engine mode toggle
		if (state.engineMode === 'remote') {
			if (radioRemote) radioRemote.checked = true;
			if (localControls) localControls.style.display = 'none';
			if (remoteControls) remoteControls.style.display = 'flex';
		} else {
			if (radioLocal) radioLocal.checked = true;
			if (localControls) localControls.style.display = 'flex';
			if (remoteControls) remoteControls.style.display = 'none';
		}

		if (remoteUrlInput && state.remoteUrl) {
			remoteUrlInput.value = state.remoteUrl;
		}
		if (remoteTokenInput) {
			remoteTokenInput.value = state.token || '';
			if (state.token) {
				remoteTokenInput.placeholder = 'Optional bearer auth token';
			} else {
				remoteTokenInput.placeholder = 'Optional bearer auth token';
			}
		}

		// Update proxy copy endpoint
		updateProxyEndpoint();

		if (state.engineStatus) {
			updateEngineStatus(state.engineStatus);
		}
		
		// Render dynamic providers from config.yaml
		renderProviders(state.providers);
	}

	function updateEngineStatus(status) {
		state.engineStatus = status;
		if (!engineStatusChip) return;
		
		engineStatusChip.className = 'status-chip';
		if (status.connected) {
			engineStatusChip.classList.add('chip-green');
			engineStatusChip.textContent = `🟢 Engine Active (${status.version || 'Online'})`;
			if (btnEngineStart) btnEngineStart.style.display = 'none';
			if (btnEngineStop) btnEngineStop.style.display = 'inline-flex';
		} else if (status.starting) {
			engineStatusChip.classList.add('chip-gray');
			engineStatusChip.textContent = '⚡ Starting Routing Engine...';
			if (btnEngineStart) btnEngineStart.style.display = 'none';
			if (btnEngineStop) btnEngineStop.style.display = 'none';
		} else if (status.testing) {
			engineStatusChip.classList.add('chip-gray');
			engineStatusChip.textContent = '⚡ Checking Connection...';
		} else {
			const isLocal = state.engineMode === 'local';
			const isConnRefused = status.error && (status.error.includes('ECONNREFUSED') || status.error.includes('Connection refused') || status.error.includes('fetch failed'));
			
			if (isLocal && (isConnRefused || !status.error || status.error === 'Offline' || status.error === 'Stopped by user')) {
				engineStatusChip.classList.add('chip-gray');
				engineStatusChip.textContent = status.error === 'Stopped by user' ? '⚪ Engine Stopped' : '⚪ Engine Offline (Click ▶️ Start)';
			} else {
				engineStatusChip.classList.add('chip-red');
				engineStatusChip.textContent = `🔴 Offline (${status.error || 'Connection refused'})`;
			}
			if (btnEngineStart) btnEngineStart.style.display = 'inline-flex';
			if (btnEngineStop) btnEngineStop.style.display = 'none';
		}
	}

	function renderProviders(providers) {
		const container = document.getElementById('providers-container');
		if (!container) return;

		if (!providers || providers.length === 0) {
			const isEngineConnected = state.engineStatus && state.engineStatus.connected;
			if (!isEngineConnected) {
				container.innerHTML = `
					<div class="partner-desc" style="text-align: center; padding: 12px 0;">
						⚪ Engine is offline.<br>
						Click <strong style="color: #60a5fa;">▶️ Start</strong> to load active model tiers and circuits.
					</div>`;
			} else {
				container.innerHTML = `
					<div class="partner-desc" style="text-align: center; padding: 12px 0;">
						No upstream providers configured in active config.<br>
						<button class="btn btn-secondary btn-compact" onclick="openConfigFile()" style="margin-top: 8px;">📝 Edit config.yaml</button>
					</div>`;
			}
			return;
		}

		container.innerHTML = providers.map(p => {
			const badgeClass = p.active ? 'chip-green' : (p.circuitState === 'open' ? 'chip-red' : 'chip-gray');
			const badgeLabel = p.active ? 'Active' : (p.circuitState === 'open' ? 'Circuit Tripped' : 'Inactive');

			let headerLinks = '';
			if (p.id === 'ollama') {
				headerLinks = `
					<span class="specs-icon" onclick="openSpecsModal()" title="View GPU & RAM hardware requirements">
						<span class="brand-logo-svg" style="width: 10px; height: 10px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z"/></svg></span>
						Specs
					</span>
					<a href="#" class="specs-icon" onclick="openExternal('https://ollama.com/download')" title="Download Ollama">
						<span class="brand-logo-svg" style="width: 10px; height: 10px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg></span>
						Download
					</a>
				`;
			} else if (p.id === 'openrouter') {
				headerLinks = `
					<a href="#" class="specs-icon" onclick="openExternal('https://openrouter.ai/keys')" title="Get API Key on OpenRouter">
						<span class="brand-logo-svg" style="width: 10px; height: 10px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M18 19H6c-.55 0-1-.45-1-1V6c0-.55.45-1 1-1h5c.55 0 1-.45 1-1s-.45-1-1-1H5c-1.11 0-2 .9-2 2v14c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2v-6c0-.55-.45-1-1-1s-1 .45-1 1v5c0 .55-.45 1-1 1zM14 4c0 .55.45 1 1 1h2.59l-9.13 9.13c-.39.39-.39 1.02 0 1.41.39.39 1.02.39 1.41 0L19 6.41V9c0 .55.45 1 1 1s1-.45 1-1V3h-6c-.55 0-1 .45-1 1z"/></svg></span>
						Get Key
					</a>
				`;
			}

			let modelInfo = '';
			if (p.models && p.models.length > 0) {
				modelInfo = `<div class="partner-desc">Configured Tier Model: <strong>${escapeHtml(p.models.join(', '))}</strong></div>`;
			} else if (p.baseUrl) {
				modelInfo = `<div class="partner-desc">Endpoint: <code>${escapeHtml(p.baseUrl)}</code></div>`;
			}

			let copyAction = '';
			if (p.id === 'ollama' && p.models && p.models.length > 0) {
				const m = p.models[0];
				copyAction = `
					<button class="btn btn-secondary btn-compact" onclick="copyOllamaCommand('${escapeHtml(m)}')">
						<span class="brand-logo-svg" style="width: 12px; height: 12px;"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/></svg></span>
						<span>Copy: ollama run ${escapeHtml(m)}</span>
					</button>
				`;
			}

			let metaText = '';
			if (p.baseUrl && p.id !== 'ollama' && p.id !== 'openrouter') {
				metaText = `<div class="partner-meta-text">Target: <code>${escapeHtml(p.baseUrl)}</code></div>`;
			} else if (p.id === 'openrouter') {
				metaText = `<div class="partner-meta-text">Live model deals & frontier cloud endpoints</div>`;
			}

			const logoSvg = getProviderLogoSvg(p.id, p.icon);

			return `
				<div class="partner-item">
					<div class="partner-header">
						<div class="partner-title">
							${logoSvg}
							<span>${escapeHtml(p.name || p.id)}</span>
							${headerLinks}
						</div>
						<div class="status-chip ${badgeClass}">
							<span class="status-dot"></span>
							<span>${badgeLabel}</span>
						</div>
					</div>
					${modelInfo}
					${metaText}
					${copyAction}
				</div>
			`;
		}).join('');
	}

	function getProviderLogoSvg(id, fallbackIcon) {
		if (id === 'ollama') {
			return `<span class="brand-logo-svg ollama" title="Ollama"><svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M10 2.5C9.45 2.5 9 2.95 9 3.5V6.1C8.2 6.4 7.5 6.9 7 7.5 6.3 8.3 6 9.3 6 10.5V14c0 .8.6 1.5 1.4 1.5H8v4.5c0 .6.4 1 1 1h2c.6 0 1-.4 1-1v-4h2v4c0 .6.4 1 1 1h2c.6 0 1-.4 1-1V15.5h.6c.8 0 1.4-.7 1.4-1.5v-3.5c0-1.2-.3-2.2-1-3-.5-.6-1.2-1.1-2-1.4V3.5c0-.55-.45-1-1-1s-1 .45-1 1v1.1c-.6-.1-1.3-.1-2-.1s-1.4 0-2 .1V3.5c0-.55-.45-1-1-1z" fill="currentColor"/><circle cx="9.5" cy="10.5" r="1.25" fill="#18181b"/><circle cx="14.5" cy="10.5" r="1.25" fill="#18181b"/><ellipse cx="12" cy="12.75" rx="1.5" ry="1" fill="#18181b"/></svg></span>`;
		}
		if (id === 'openrouter') {
			return `<span class="brand-logo-svg openrouter" title="OpenRouter"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M18.654 3.87a5.087 5.087 0 1 1 0 10.174L23.7 19.09c.64.641.187 1.737-.72 1.737H8.48a8.479 8.479 0 0 1 0-16.958h10.174zM8.48 7.262a5.087 5.087 0 1 0 0 10.175h11.23l-3.393-3.394a5.087 5.087 0 0 1-7.837-6.781z"/></svg></span>`;
		}
		if (id === 'vllm') {
			return `<span class="brand-logo-svg vllm" title="vLLM"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M0 4.973h9.324V23L0 4.973z"/><path d="M13.986 4.351L22.378 0l-8.392 18.027 8.392 5.973H13.986L5.595 18.027 13.986 4.351z"/></svg></span>`;
		}
		return `<span class="brand-logo-svg">${fallbackIcon || '🔌'}</span>`;
	}

	function escapeHtml(text) {
		if (!text) return '';
		return text.replace(/[&<>"']/g, m => {
			switch (m) {
				case '&': return '&amp;';
				case '<': return '&lt;';
				case '>': return '&gt;';
				case '"': return '&quot;';
				case "'": return '&#39;';
				default: return m;
			}
		});
	}

	function updateProxyEndpoint() {
		if (proxyEndpointText) {
			if (state.engineMode === 'remote' && state.remoteUrl) {
				try {
					const url = new URL(state.remoteUrl);
					proxyEndpointText.textContent = `${url.origin}/v1`;
				} catch (_) {
					proxyEndpointText.textContent = `${state.remoteUrl}/v1`;
				}
			} else {
				proxyEndpointText.textContent = 'http://127.0.0.1:8000/v1';
			}
		}

		const tokenTextEl = document.getElementById('agent-token-text');
		if (tokenTextEl) {
			if (state.hasToken || state.token) {
				tokenTextEl.textContent = '•••••••• (Active Token)';
				tokenTextEl.style.color = '#93c5fd';
			} else {
				tokenTextEl.textContent = '(No auth required)';
				tokenTextEl.style.color = 'var(--vscode-descriptionForeground)';
			}
		}
	}

	// Global Action Handlers
	window.setEngineMode = function(mode) {
		state.engineMode = mode;
		applyState(state);
		vscode.postMessage({ command: 'setEngineMode', mode });
	};

	window.copyActiveToken = function() {
		vscode.postMessage({ command: 'copyActiveToken' });
	};

	window.copyModelId = function() {
		vscode.postMessage({ command: 'copyToClipboard', text: 'nacho-hybrid', label: 'Model ID (nacho-hybrid)' });
	};

	window.startEngine = function() {
		vscode.postMessage({ command: 'startEngine' });
	};

	window.restartEngine = function() {
		vscode.postMessage({ command: 'restartEngine' });
	};

	window.stopEngine = function() {
		vscode.postMessage({ command: 'stopEngine' });
	};

	window.openEngineLogs = function() {
		vscode.postMessage({ command: 'openLogs' });
	};

	window.openConfigFile = function() {
		vscode.postMessage({ command: 'editConfig' });
	};

	window.testRemoteConnection = function() {
		const url = remoteUrlInput ? remoteUrlInput.value.trim() : '';
		const token = remoteTokenInput ? remoteTokenInput.value.trim() : '';
		updateEngineStatus({ testing: true });
		vscode.postMessage({ command: 'testConnection', url, token });
	};

	window.saveRemoteSettings = function() {
		const url = remoteUrlInput ? remoteUrlInput.value.trim() : '';
		const token = remoteTokenInput ? remoteTokenInput.value : '';
		vscode.postMessage({ command: 'saveEngineSettings', url, token });
	};

	window.saveOpenRouterKey = function() {
		const key = openrouterKeyInput ? openrouterKeyInput.value.trim() : '';
		vscode.postMessage({ command: 'saveOpenRouterKey', key });
	};

	window.togglePasswordVisibility = function(inputId, btnId) {
		const input = document.getElementById(inputId);
		const btn = document.getElementById(btnId);
		if (!input || !btn) return;
		if (input.type === 'password') {
			input.type = 'text';
			btn.textContent = '🔒';
		} else {
			input.type = 'password';
			btn.textContent = '👁️';
		}
	};

	window.copyZooEndpoint = function() {
		const text = proxyEndpointText ? proxyEndpointText.textContent : 'http://127.0.0.1:8000/v1';
		vscode.postMessage({ command: 'copyToClipboard', text, label: 'Zoo Code Proxy Endpoint' });
	};
	window.copyRooEndpoint = window.copyZooEndpoint;

	window.copyOllamaCommand = function(modelName) {
		let model = modelName;
		if (!model) {
			const ollamaP = (state.providers || []).find(p => p.id === 'ollama');
			model = (ollamaP && ollamaP.models && ollamaP.models.length > 0) ? ollamaP.models[0] : 'gemma4:12b-it-qat';
		}
		vscode.postMessage({ command: 'copyToClipboard', text: `ollama run ${model}`, label: `Ollama pull command (${model})` });
	};

	window.openExternal = function(url) {
		vscode.postMessage({ command: 'openExternalUrl', url });
	};

	window.openMarketplace = function(extensionId) {
		vscode.postMessage({ command: 'openMarketplace', extensionId });
	};

	window.openSpecsModal = function() {
		if (specsModal) specsModal.style.display = 'flex';
	};

	window.closeSpecsModal = function() {
		if (specsModal) specsModal.style.display = 'none';
	};

	window.recalculateStats = function() {
		vscode.postMessage({ command: 'recalculateStats' });
	};

	window.resetCircuits = function() {
		vscode.postMessage({ command: 'resetCircuits' });
	};

	window.confirmResetStats = function() {
		if (dangerModal) dangerModal.style.display = 'flex';
	};

	window.closeDangerModal = function() {
		if (dangerModal) dangerModal.style.display = 'none';
	};

	window.executeResetStats = function() {
		window.closeDangerModal();
		vscode.postMessage({ command: 'resetStats' });
	};

	window.openDashboard = function() {
		vscode.postMessage({ command: 'openDashboard' });
	};

	window.openDocsUrl = function() {
		vscode.postMessage({ command: 'openDocs' });
	};

	window.openSupportUrl = function() {
		vscode.postMessage({ command: 'openSupport' });
	};

	window.refreshAll = function() {
		vscode.postMessage({ command: 'refreshAll' });
	};

	// Initialize
	document.addEventListener('DOMContentLoaded', () => {
		vscode.postMessage({ command: 'initialize' });
	});
})();
