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
			const badgeText = p.active ? '🟢 Active' : (p.circuitState === 'open' ? '🔴 Circuit Tripped' : '⚪ Inactive');

			let headerLinks = '';
			if (p.id === 'ollama') {
				headerLinks = `
					<span class="specs-icon" onclick="openSpecsModal()" title="View GPU & RAM hardware requirements">ℹ️ Specs</span>
					<a href="#" class="specs-icon" onclick="openExternal('https://ollama.com/download')" title="Download Ollama">📥 Download</a>
				`;
			} else if (p.id === 'openrouter') {
				headerLinks = `
					<a href="#" class="specs-icon" onclick="openExternal('https://openrouter.ai/keys')" title="Get API Key on OpenRouter">🔗 Get Key</a>
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
				copyAction = `<button class="btn btn-secondary btn-compact" onclick="copyOllamaCommand('${escapeHtml(m)}')">📋 Copy: ollama run ${escapeHtml(m)}</button>`;
			}

			let metaText = '';
			if (p.baseUrl && p.id !== 'ollama' && p.id !== 'openrouter') {
				metaText = `<div class="partner-meta-text">Target: <code>${escapeHtml(p.baseUrl)}</code></div>`;
			} else if (p.id === 'openrouter') {
				metaText = `<div class="partner-meta-text">Spot market arbitrage & frontier cloud models</div>`;
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
						<div class="status-chip ${badgeClass}">${badgeText}</div>
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
			return `<span class="brand-logo-svg ollama" title="Ollama"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M16.361 10.26a.894.894 0 0 0-.558.47l-.072.148.001.207c0 .193.004.217.059.353.076.193.152.312.291.448.24.238.51.3.872.205a.86.86 0 0 0 .517-.436.752.752 0 0 0 .08-.498c-.064-.453-.33-.782-.724-.897a1.06 1.06 0 0 0-.466 0zm-9.203.005c-.305.096-.533.32-.65.639a1.187 1.187 0 0 0-.06.52c.057.309.31.59.598.667.362.095.632.033.872-.205.14-.136.215-.255.291-.448.055-.136.059-.16.059-.353l.001-.207-.072-.148a.894.894 0 0 0-.565-.472 1.02 1.02 0 0 0-.474.007Zm4.184 2c-.131.071-.223.25-.195.383.031.143.157.288.353.407.105.063.112.072.117.136.004.038-.01.146-.029.243-.02.094-.036.194-.036.222.002.074.07.195.143.253.064.052.076.054.255.059.164.005.198.001.264-.03.169-.082.212-.234.15-.525-.052-.243-.042-.28.087-.355.137-.08.281-.219.324-.314a.365.365 0 0 0-.175-.48.394.394 0 0 0-.181-.033c-.126 0-.207.03-.355.124l-.085.053-.053-.032c-.219-.13-.259-.145-.391-.143a.396.396 0 0 0-.193.032zm.39-2.195c-.373.036-.475.05-.654.086-.291.06-.68.195-.951.328-.94.46-1.589 1.226-1.787 2.114-.04.176-.045.234-.045.53 0 .294.005.357.043.524.264 1.16 1.332 2.017 2.714 2.173.3.033 1.596.033 1.896 0 1.11-.125 2.064-.727 2.493-1.571.114-.226.169-.372.22-.602.039-.167.044-.23.044-.523 0-.297-.005-.355-.045-.531-.288-1.29-1.539-2.304-3.072-2.497a6.873 6.873 0 0 0-.855-.031zm.645.937a3.283 3.283 0 0 1 1.44.514c.223.148.537.458.671.662.166.251.26.508.303.82.02.143.01.251-.043.482-.08.345-.332.705-.672.957a3.115 3.115 0 0 1-.689.348c-.382.122-.632.144-1.525.138-.582-.006-.686-.01-.853-.042-.57-.107-1.022-.334-1.35-.68-.264-.28-.385-.535-.45-.946-.03-.192.025-.509.137-.776.136-.326.488-.73.836-.963.403-.269.934-.46 1.422-.512.187-.02.586-.02.773-.002zm-5.503-11a1.653 1.653 0 0 0-.683.298C5.617.74 5.173 1.666 4.985 2.819c-.07.436-.119 1.04-.119 1.503 0 .544.064 1.24.155 1.721.02.107.031.202.023.208a8.12 8.12 0 0 1-.187.152 5.324 5.324 0 0 0-.949 1.02 5.49 5.49 0 0 0-.94 2.339 6.625 6.625 0 0 0-.023 1.357c.091.78.325 1.438.727 2.04l.13.195-.037.064c-.269.452-.498 1.105-.605 1.732-.084.496-.095.629-.095 1.294 0 .67.009.803.088 1.266.095.555.288 1.143.503 1.534.071.128.243.393.264.407.007.003-.014.067-.046.141a7.405 7.405 0 0 0-.548 1.873c-.062.417-.071.552-.071.991 0 .56.031.832.148 1.279L3.42 24h1.478l.056-.51c.074-.668.271-1.332.583-1.956.126-.251.341-.572.483-.715.111-.112.18-.14.475-.19.336-.057.48-.052.793.029.28.072.58.21.84.385.297.2.627.535.811.825.267.42.441.884.542 1.455.034.195.056.407.075.677H10.45l.05-.444c.08-.722.28-1.424.604-2.072.138-.276.388-.636.568-.82.164-.167.3-.23.633-.292.428-.08.665-.037 1.05.195.344.207.69.57.854.896.257.513.407 1.084.484 1.848l.044.689h1.493l.035-.386c.071-.786.275-1.528.618-2.22.14-.282.404-.66.593-.852.164-.166.304-.23.637-.291.432-.08.665-.037 1.053.197.34.205.69.569.851.892.257.514.407 1.087.485 1.854l.044.806h1.488l.071-.444a6.685 6.685 0 0 0 .15-.99c0-.439-.009-.574-.071-.991a7.405 7.405 0 0 0-.548-1.873c-.032-.074-.053-.138-.046-.141.021-.014.193-.279.264-.407.215-.391.408-.979.503-1.534.079-.463.088-.596.088-1.266 0-.665-.011-.798-.095-1.294a6.993 6.993 0 0 0-.605-1.732l-.037-.064.13-.195a5.556 5.556 0 0 0 .727-2.04 6.625 6.625 0 0 0-.023-1.357 5.49 5.49 0 0 0-.94-2.339 5.324 5.324 0 0 0-.949-1.02 8.12 8.12 0 0 1-.187-.152c-.008-.006.003-.101.023-.208.091-.481.155-1.177.155-1.721 0-.463-.049-1.067-.119-1.503C18.827 1.666 18.383.74 17.702.298a1.653 1.653 0 0 0-.683-.298c-.282-.047-.732.053-1.088.243-.377.202-.75.568-.978.96-.22.378-.396.903-.497 1.488-.043.25-.062.457-.065.708l-.004.306-.217-.065a7.195 7.195 0 0 0-2.172-.34c-.753 0-1.52.12-2.172.34l-.217.065-.004-.306c-.003-.251-.022-.458-.065-.708-.101-.585-.277-1.11-.497-1.488-.228-.392-.601-.758-.978-.96-.356-.19-.806-.29-1.088-.243z"/></svg></span>`;
		}
		if (id === 'openrouter') {
			return `<span class="brand-logo-svg openrouter" title="OpenRouter"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M18.654 3.87a5.087 5.087 0 110 10.174L23.7 19.09c.64.641.187 1.737-.72 1.737H8.48a8.479 8.479 0 010-16.958h10.174zM8.48 7.262a5.087 5.087 0 100 10.175h11.23l-3.393-3.394a5.087 5.087 0 01-7.837-6.781z"/></svg></span>`;
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

	window.copyRooEndpoint = function() {
		const text = proxyEndpointText ? proxyEndpointText.textContent : 'http://127.0.0.1:8000/v1';
		vscode.postMessage({ command: 'copyToClipboard', text, label: 'Roo Code Proxy Endpoint' });
	};

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

	window.refreshAll = function() {
		vscode.postMessage({ command: 'refreshAll' });
	};

	// Initialize
	document.addEventListener('DOMContentLoaded', () => {
		vscode.postMessage({ command: 'initialize' });
	});
})();
