package main

func getOnboardingScript() string {
	return `
		(function() {
			var ONBOARDING_KEY = 'whatsapp_desktop_onboarded_v4';

			function shortcut(keys) {
				return '<kbd style="font:500 11px/1.2 ui-monospace,SFMono-Regular,Consolas,monospace;color:inherit;background:transparent;border:1px solid currentColor;border-radius:4px;padding:3px 6px;opacity:.72;white-space:nowrap;">' + keys + '</kbd>';
			}

			function showOnboarding(force) {
				if (!force && localStorage.getItem(ONBOARDING_KEY) === 'true') return;
				if (document.getElementById('wa-onboarding-overlay')) return;

				if (!document.getElementById('wa-onboarding-style')) {
					var style = document.createElement('style');
					style.id = 'wa-onboarding-style';
					style.textContent = '' +
						'@keyframes waOnboardIn{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}' +
						'.wa-onboard-action:focus-visible,.wa-onboard-skip:focus-visible{outline:2px solid #00a884;outline-offset:2px}' +
						'.wa-onboard-action:hover{background:#017561!important}' +
						'.wa-onboard-skip:hover{text-decoration:underline}' +
						'@media(prefers-reduced-motion:reduce){#wa-onboarding-panel{animation:none!important}#wa-onboarding-overlay{transition:none!important}}';
					document.head.appendChild(style);
				}

				var dark = document.documentElement.classList.contains('dark') || document.body.classList.contains('dark');
				var surface = dark ? '#111b21' : '#f7f9fa';
				var text = dark ? '#e9edef' : '#111b21';
				var muted = dark ? '#9aa8b0' : '#667781';
				var border = dark ? '#33434c' : '#d8dfe3';

				var overlay = document.createElement('div');
				overlay.id = 'wa-onboarding-overlay';
				overlay.style.cssText = 'position:fixed;inset:0;background:rgba(8,15,19,.68);z-index:999999;display:flex;align-items:center;justify-content:center;padding:20px;box-sizing:border-box;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;color:' + text + ';opacity:0;transition:opacity 160ms cubic-bezier(.16,1,.3,1);';

				var panel = document.createElement('section');
				panel.id = 'wa-onboarding-panel';
				panel.setAttribute('role', 'dialog');
				panel.setAttribute('aria-modal', 'true');
				panel.setAttribute('aria-labelledby', 'wa-onboarding-title');
				panel.style.cssText = 'width:440px;max-width:100%;background:' + surface + ';border:1px solid ' + border + ';border-radius:10px;box-shadow:0 18px 48px rgba(0,0,0,.32);padding:24px;box-sizing:border-box;animation:waOnboardIn 180ms cubic-bezier(.16,1,.3,1);';

				var intro = document.createElement('div');
				intro.style.cssText = 'margin-bottom:20px;';
				intro.innerHTML = '<div style="display:flex;align-items:center;gap:10px;margin-bottom:8px;">' +
					'<span style="width:10px;height:10px;border-radius:50%;background:#00a884;flex:none;"></span>' +
					'<h1 id="wa-onboarding-title" style="font-size:18px;line-height:1.3;font-weight:600;letter-spacing:-.15px;margin:0;">WhatsApp Desk is ready</h1>' +
					'</div>' +
					'<p style="font-size:13px;line-height:1.55;color:' + muted + ';margin:0;max-width:58ch;">Log in or scan QR code as usual. Desktop controls and shortcuts are available whenever you need them.</p>';
				panel.appendChild(intro);

				var rows = document.createElement('div');
				rows.style.cssText = 'border-top:1px solid ' + border + ';border-bottom:1px solid ' + border + ';margin-bottom:20px;';
				var items = [
					['Blur conversation messages', 'Cmd/Ctrl + Shift + P'],
					['Keep window always on top', 'Cmd/Ctrl + Shift + T'],
					['Open settings & controls', 'Cmd/Ctrl + ,']
				];
				items.forEach(function(item, index) {
					var row = document.createElement('div');
					row.style.cssText = 'min-height:44px;display:flex;align-items:center;justify-content:space-between;gap:16px;' + (index ? 'border-top:1px solid ' + border + ';' : '');
					row.innerHTML = '<span style="font-size:12.5px;line-height:1.4;">' + item[0] + '</span>' + shortcut(item[1]);
					rows.appendChild(row);
				});
				panel.appendChild(rows);

				var footer = document.createElement('div');
				footer.style.cssText = 'display:flex;align-items:center;justify-content:space-between;gap:12px;';
				var skip = document.createElement('button');
				skip.className = 'wa-onboard-skip';
				skip.textContent = 'Skip guide';
				skip.style.cssText = 'background:transparent;border:0;color:' + muted + ';font-size:12px;padding:7px 0;cursor:pointer;';
				var continueButton = document.createElement('button');
				continueButton.className = 'wa-onboard-action';
				continueButton.textContent = 'Continue to WhatsApp';
				continueButton.style.cssText = 'background:#008069;color:#f7f9fa;border:0;border-radius:6px;padding:8px 14px;font-size:12.5px;font-weight:600;cursor:pointer;transition:background 160ms cubic-bezier(.16,1,.3,1);';
				footer.appendChild(skip);
				footer.appendChild(continueButton);
				panel.appendChild(footer);

				function dismiss() {
					localStorage.setItem(ONBOARDING_KEY, 'true');
					overlay.style.opacity = '0';
					setTimeout(function() {
						if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
					}, 170);
				}
				skip.onclick = dismiss;
				continueButton.onclick = dismiss;
				overlay.addEventListener('keydown', function(e) { if (e.key === 'Escape') dismiss(); });

				overlay.appendChild(panel);
				document.body.appendChild(overlay);
				requestAnimationFrame(function() { overlay.style.opacity = '1'; continueButton.focus(); });
			}

			window.showOnboardingModal = function() { showOnboarding(true); };
			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'h' || e.key === 'H')) {
					e.preventDefault();
					showOnboarding(true);
				}
			});

			function initCheck() {
				if (!document.body) { setTimeout(initCheck, 150); return; }
				setTimeout(function() { showOnboarding(false); }, 500);
			}
			initCheck();
		})();
	`
}
