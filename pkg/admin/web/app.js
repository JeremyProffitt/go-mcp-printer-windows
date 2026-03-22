// Go MCP Printer - Admin UI

const App = {
    currentPage: 'config',

    init() {
        this.bindNav();
        this.loadPage(location.hash.slice(1) || 'config');
        window.addEventListener('hashchange', () => {
            this.loadPage(location.hash.slice(1) || 'config');
        });
    },

    bindNav() {
        document.querySelectorAll('.sidebar nav a').forEach(a => {
            a.addEventListener('click', (e) => {
                e.preventDefault();
                const page = a.dataset.page;
                location.hash = page;
            });
        });
    },

    loadPage(page) {
        this.currentPage = page;
        document.querySelectorAll('.sidebar nav a').forEach(a => {
            a.classList.toggle('active', a.dataset.page === page);
        });

        const main = document.querySelector('.main');
        switch (page) {
            case 'config': this.showConfig(main); break;
            case 'printers': this.showPrinters(main); break;
            case 'logs': this.showLogs(main); break;
            case 'status': this.showStatus(main); break;
            default: this.showConfig(main);
        }
    },

    async showConfig(el) {
        el.innerHTML = '<div class="card"><h2>Server Configuration</h2><div id="config-form">Loading...</div></div>';
        try {
            const [cfgResp, printersResp] = await Promise.all([
                fetch('/admin/api/config'),
                fetch('/admin/api/printers'),
            ]);
            const cfg = await cfgResp.json();
            const printers = printersResp.ok ? await printersResp.json() : [];
            document.getElementById('config-form').innerHTML = this.renderConfigForm(cfg, printers);
            document.getElementById('save-config').addEventListener('click', () => this.saveConfig());
            document.getElementById('restart-server').addEventListener('click', () => this.restartServer());
            document.getElementById('cfg-allowAll').addEventListener('change', (e) => {
                const items = document.querySelectorAll('input[name="allowedPrinter"]');
                items.forEach(cb => { cb.disabled = e.target.checked; cb.checked = e.target.checked; });
                document.querySelectorAll('#cfg-allowedPrinters .checklist-printer').forEach(el => {
                    el.style.opacity = e.target.checked ? '0.5' : '1';
                });
            });
        } catch (e) {
            document.getElementById('config-form').innerHTML = '<p class="error">Failed to load config</p>';
        }
    },

    renderConfigForm(cfg, printers) {
        const defaultName = cfg.defaultPrinter || '';
        const systemDefault = (printers || []).find(p => p.isDefault);
        const selectedValue = defaultName || (systemDefault ? systemDefault.name : '');

        return `
            <div class="form-row">
                <div class="form-group">
                    <label>Domain</label>
                    <input type="text" id="cfg-domain" value="${cfg.domain || ''}" placeholder="printer.example.com">
                </div>
                <div class="form-group">
                    <label>Port</label>
                    <input type="number" id="cfg-port" value="${cfg.port || 80}">
                </div>
            </div>
            <div class="form-group">
                <label>Log Level</label>
                <select id="cfg-logLevel">
                    ${['off','error','warn','info','access','debug'].map(l =>
                        `<option value="${l}" ${cfg.logLevel === l ? 'selected' : ''}>${l}</option>`
                    ).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>Default Printer</label>
                <select id="cfg-defaultPrinter">
                    <option value="">(System Default)</option>
                    ${(printers || []).map(p =>
                        `<option value="${p.name}" ${p.name === selectedValue ? 'selected' : ''}>${p.name}${p.isDefault ? ' (system default)' : ''}</option>`
                    ).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>Allowed Printers</label>
                <div class="printer-checklist" id="cfg-allowedPrinters">
                    <label class="checklist-item">
                        <input type="checkbox" id="cfg-allowAll" ${(cfg.allowedPrinters || []).length === 0 ? 'checked' : ''}> <strong>Allow All Printers</strong>
                    </label>
                    ${(printers || []).map(p => {
                        const checked = (cfg.allowedPrinters || []).length === 0 || (cfg.allowedPrinters || []).includes(p.name);
                        return `<label class="checklist-item checklist-printer" ${(cfg.allowedPrinters || []).length === 0 ? 'style="opacity:0.5"' : ''}>
                            <input type="checkbox" name="allowedPrinter" value="${p.name}" ${checked ? 'checked' : ''} ${(cfg.allowedPrinters || []).length === 0 ? 'disabled' : ''}> ${p.name}
                        </label>`;
                    }).join('')}
                </div>
            </div>
            <div class="form-group">
                <label>Blocked Printers</label>
                <div class="printer-checklist" id="cfg-blockedPrinters">
                    ${(printers || []).length === 0 ? '<span style="color:var(--text-muted);font-size:13px">No printers found</span>' :
                    (printers || []).map(p => {
                        const checked = (cfg.blockedPrinters || []).includes(p.name);
                        return `<label class="checklist-item">
                            <input type="checkbox" name="blockedPrinter" value="${p.name}" ${checked ? 'checked' : ''}> ${p.name}
                        </label>`;
                    }).join('')}
                </div>
            </div>
            <div class="form-group">
                <label>Photo Printers <span style="font-weight:normal;color:var(--text-muted)">(dye-sub / photo printers)</span></label>
                <div class="printer-checklist" id="cfg-photoPrinters">
                    ${(printers || []).length === 0 ? '<span style="color:var(--text-muted);font-size:13px">No printers found</span>' :
                    (printers || []).map(p => {
                        const checked = (cfg.photoPrinters || []).includes(p.name);
                        return `<label class="checklist-item">
                            <input type="checkbox" name="photoPrinter" value="${p.name}" ${checked ? 'checked' : ''}> ${p.name}
                        </label>`;
                    }).join('')}
                </div>
            </div>
            <div class="form-group">
                <label>Allowed Paths (comma-separated, empty = all)</label>
                <input type="text" id="cfg-allowedPaths" value="${(cfg.allowedPaths || []).join(', ')}">
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Rate Limit (calls)</label>
                    <input type="number" id="cfg-rateLimitCalls" value="${cfg.rateLimitCalls || 10}">
                </div>
                <div class="form-group">
                    <label>Rate Limit Window (seconds)</label>
                    <input type="number" id="cfg-rateLimitWindow" value="${cfg.rateLimitWindow || 20}">
                </div>
            </div>
            <button id="save-config" class="btn btn-primary">Save Configuration</button>
            <button id="restart-server" class="btn btn-secondary" style="margin-left:8px">Restart Server</button>
        `;
    },

    async restartServer() {
        try {
            const resp = await fetch('/admin/api/restart', { method: 'POST' });
            if (resp.ok) {
                this.toast('Server restarting...', 'success');
                // Poll until the server comes back
                setTimeout(() => this.waitForServer(), 2000);
            } else {
                this.toast('Failed to restart', 'error');
            }
        } catch (e) {
            this.toast('Restart sent, waiting for server...', 'success');
            setTimeout(() => this.waitForServer(), 2000);
        }
    },

    async waitForServer(attempts) {
        attempts = attempts || 0;
        if (attempts > 15) {
            this.toast('Server did not come back. Check admin port in config.', 'error');
            return;
        }
        try {
            const resp = await fetch('/admin/api/status');
            if (resp.ok) {
                this.toast('Server is back online', 'success');
                this.loadPage(this.currentPage);
                return;
            }
        } catch (e) { /* still restarting */ }
        setTimeout(() => this.waitForServer(attempts + 1), 1000);
    },

    async saveConfig() {
        const splitList = (v) => v ? v.split(',').map(s => s.trim()).filter(Boolean) : [];
        const checkedValues = (name) => [...document.querySelectorAll(`input[name="${name}"]:checked`)].map(cb => cb.value);
        const allowAll = document.getElementById('cfg-allowAll').checked;
        const cfg = {
            domain: document.getElementById('cfg-domain').value,
            port: parseInt(document.getElementById('cfg-port').value) || 80,
            logLevel: document.getElementById('cfg-logLevel').value,
            defaultPrinter: document.getElementById('cfg-defaultPrinter').value,
            allowedPrinters: allowAll ? [] : checkedValues('allowedPrinter'),
            blockedPrinters: checkedValues('blockedPrinter'),
            photoPrinters: checkedValues('photoPrinter'),
            allowedPaths: splitList(document.getElementById('cfg-allowedPaths').value),
            rateLimitCalls: parseInt(document.getElementById('cfg-rateLimitCalls').value) || 10,
            rateLimitWindow: parseInt(document.getElementById('cfg-rateLimitWindow').value) || 20,
        };

        try {
            const resp = await fetch('/admin/api/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(cfg),
            });
            if (resp.ok) {
                const result = await resp.json();
                if (result.restart) {
                    this.toast('Configuration saved. Server restarting on new port...', 'success');
                    setTimeout(() => this.waitForServer(), 2000);
                } else {
                    this.toast('Configuration saved', 'success');
                }
            } else {
                this.toast('Failed to save', 'error');
            }
        } catch (e) {
            this.toast('Error: ' + e.message, 'error');
        }
    },

    async showPrinters(el) {
        el.innerHTML = `
            <div class="card">
                <h2>Printers</h2>
                <div style="margin-bottom:16px">
                    <button id="test-all-btn" class="btn btn-primary">Print Test Page on All Printers</button>
                    <button id="show-paper-btn" class="btn btn-secondary" style="margin-left:8px">Show Paper Sizes</button>
                </div>
                <div id="printer-list">Loading...</div>
            </div>
            <div id="paper-sizes-card" style="display:none" class="card">
                <h2>Paper Sizes by Printer</h2>
                <div id="paper-sizes">Loading...</div>
            </div>`;

        document.getElementById('test-all-btn').addEventListener('click', () => this.testAllPrinters());
        document.getElementById('show-paper-btn').addEventListener('click', () => this.showPaperSizes());

        try {
            const resp = await fetch('/admin/api/printers');
            const printers = await resp.json();
            if (!printers || printers.length === 0) {
                document.getElementById('printer-list').innerHTML = '<p>No printers found</p>';
                return;
            }
            let html = '<table><thead><tr><th>Name</th><th>Driver</th><th>Status</th><th>Default</th><th>Type</th></tr></thead><tbody>';
            for (const p of printers) {
                const badge = p.printerState === 'Normal' ? 'badge-success' : 'badge-warning';
                html += `<tr>
                    <td>${p.name}</td>
                    <td>${p.driverName || '-'}</td>
                    <td><span class="badge ${badge}">${p.printerState}</span></td>
                    <td>${p.isDefault ? 'Yes' : ''}</td>
                    <td>${p.type || 'local'}</td>
                </tr>`;
            }
            html += '</tbody></table>';
            document.getElementById('printer-list').innerHTML = html;
        } catch (e) {
            document.getElementById('printer-list').innerHTML = '<p class="error">Failed to load printers</p>';
        }
    },

    async testAllPrinters() {
        this.toast('Sending test pages to all printers...', 'success');
        try {
            const resp = await fetch('/admin/api/printers/test-all', { method: 'POST' });
            const results = await resp.json();
            let msg = 'Test page results:\n';
            for (const [name, result] of Object.entries(results)) {
                msg += `${name}: ${result}\n`;
            }
            this.toast(msg, resp.ok ? 'success' : 'error');
        } catch (e) {
            this.toast('Error: ' + e.message, 'error');
        }
    },

    async showPaperSizes() {
        const card = document.getElementById('paper-sizes-card');
        card.style.display = 'block';
        document.getElementById('paper-sizes').innerHTML = 'Loading paper sizes (this may take a moment)...';

        try {
            const resp = await fetch('/admin/api/printers/paper-sizes');
            const printers = await resp.json();
            if (!printers || printers.length === 0) {
                document.getElementById('paper-sizes').innerHTML = '<p>No printers found</p>';
                return;
            }

            let html = '';
            for (const p of printers) {
                html += `<h3 style="margin-top:16px">${p.name} ${p.isDefault ? '<span class="badge badge-success">Default</span>' : ''}</h3>`;
                if (!p.paperSizes || p.paperSizes.length === 0) {
                    html += '<p style="color:var(--text-muted)">No paper sizes reported</p>';
                    continue;
                }
                html += '<table><thead><tr><th>Paper Size</th><th>Width (mm)</th><th>Height (mm)</th><th>Width (in)</th><th>Height (in)</th></tr></thead><tbody>';
                for (const s of p.paperSizes) {
                    html += `<tr><td>${s.name}</td><td>${s.widthMm}</td><td>${s.heightMm}</td><td>${s.widthIn}</td><td>${s.heightIn}</td></tr>`;
                }
                html += '</tbody></table>';
            }
            document.getElementById('paper-sizes').innerHTML = html;
        } catch (e) {
            document.getElementById('paper-sizes').innerHTML = '<p class="error">Failed to load paper sizes</p>';
        }
    },

    async showLogs(el) {
        el.innerHTML = '<div class="card"><h2>Server Logs</h2><div style="margin-bottom:12px"><button id="clear-logs" class="btn btn-secondary">Clear Logs</button></div><div id="log-content" class="log-viewer">Loading...</div></div>';
        document.getElementById('clear-logs').addEventListener('click', () => this.clearLogs());
        try {
            const resp = await fetch('/admin/api/logs');
            const text = await resp.text();
            document.getElementById('log-content').textContent = text || 'No logs available';
        } catch (e) {
            document.getElementById('log-content').textContent = 'Failed to load logs';
        }
    },

    async clearLogs() {
        try {
            const resp = await fetch('/admin/api/logs', { method: 'DELETE' });
            if (resp.ok) {
                this.toast('Logs cleared', 'success');
                this.showLogs(document.querySelector('.main'));
            } else {
                this.toast('Failed to clear logs', 'error');
            }
        } catch (e) {
            this.toast('Error: ' + e.message, 'error');
        }
    },

    async showStatus(el) {
        el.innerHTML = '<div class="card"><h2>Server Status</h2><div id="status-info">Loading...</div><div style="margin-top:16px"><button id="status-restart" class="btn btn-secondary">Restart Server</button></div></div>';
        document.getElementById('status-restart').addEventListener('click', () => this.restartServer());
        try {
            const resp = await fetch('/admin/api/status');
            const status = await resp.json();
            let html = '<table>';
            for (const [key, value] of Object.entries(status)) {
                html += `<tr><td><strong>${key}</strong></td><td>${value}</td></tr>`;
            }
            html += '</table>';

            document.getElementById('status-info').innerHTML = html;
        } catch (e) {
            document.getElementById('status-info').innerHTML = '<p class="error">Failed to load status</p>';
        }
    },

    toast(message, type) {
        const el = document.createElement('div');
        el.className = `toast toast-${type}`;
        el.textContent = message;
        document.body.appendChild(el);
        setTimeout(() => el.remove(), 3000);
    }
};

document.addEventListener('DOMContentLoaded', () => App.init());
