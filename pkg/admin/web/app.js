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
            case 'oauth': this.showOAuth(main); break;
            case 'logs': this.showLogs(main); break;
            case 'status': this.showStatus(main); break;
            default: this.showConfig(main);
        }
    },

    async showConfig(el) {
        el.innerHTML = '<div class="card"><h2>Server Configuration</h2><div id="config-form">Loading...</div></div>';
        try {
            const resp = await fetch('/admin/api/config');
            const cfg = await resp.json();
            document.getElementById('config-form').innerHTML = this.renderConfigForm(cfg);
            document.getElementById('save-config').addEventListener('click', () => this.saveConfig());
        } catch (e) {
            document.getElementById('config-form').innerHTML = '<p class="error">Failed to load config</p>';
        }
    },

    renderConfigForm(cfg) {
        return `
            <div class="form-row">
                <div class="form-group">
                    <label>Domain</label>
                    <input type="text" id="cfg-domain" value="${cfg.domain || ''}" placeholder="printer.example.com">
                </div>
                <div class="form-group">
                    <label>HTTPS Port</label>
                    <input type="number" id="cfg-httpsPort" value="${cfg.httpsPort || 443}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>HTTP Port</label>
                    <input type="number" id="cfg-httpPort" value="${cfg.httpPort || 80}">
                </div>
                <div class="form-group">
                    <label>Log Level</label>
                    <select id="cfg-logLevel">
                        ${['off','error','warn','info','access','debug'].map(l =>
                            `<option value="${l}" ${cfg.logLevel === l ? 'selected' : ''}>${l}</option>`
                        ).join('')}
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label>ACME Email (Let's Encrypt)</label>
                <input type="email" id="cfg-acmeEmail" value="${cfg.acmeEmail || ''}" placeholder="admin@example.com">
            </div>
            <div class="form-group">
                <label><input type="checkbox" id="cfg-useSelfSigned" ${cfg.useSelfSigned ? 'checked' : ''}> Use Self-Signed Certificate</label>
            </div>
            <div class="form-group">
                <label>Default Printer</label>
                <input type="text" id="cfg-defaultPrinter" value="${cfg.defaultPrinter || ''}">
            </div>
            <div class="form-group">
                <label>Allowed Printers (comma-separated, empty = all)</label>
                <input type="text" id="cfg-allowedPrinters" value="${(cfg.allowedPrinters || []).join(', ')}">
            </div>
            <div class="form-group">
                <label>Blocked Printers (comma-separated)</label>
                <input type="text" id="cfg-blockedPrinters" value="${(cfg.blockedPrinters || []).join(', ')}">
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
        `;
    },

    async saveConfig() {
        const splitList = (v) => v ? v.split(',').map(s => s.trim()).filter(Boolean) : [];
        const cfg = {
            domain: document.getElementById('cfg-domain').value,
            httpsPort: parseInt(document.getElementById('cfg-httpsPort').value) || 443,
            httpPort: parseInt(document.getElementById('cfg-httpPort').value) || 80,
            logLevel: document.getElementById('cfg-logLevel').value,
            acmeEmail: document.getElementById('cfg-acmeEmail').value,
            useSelfSigned: document.getElementById('cfg-useSelfSigned').checked,
            defaultPrinter: document.getElementById('cfg-defaultPrinter').value,
            allowedPrinters: splitList(document.getElementById('cfg-allowedPrinters').value),
            blockedPrinters: splitList(document.getElementById('cfg-blockedPrinters').value),
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
                this.toast('Configuration saved', 'success');
            } else {
                this.toast('Failed to save', 'error');
            }
        } catch (e) {
            this.toast('Error: ' + e.message, 'error');
        }
    },

    async showPrinters(el) {
        el.innerHTML = '<div class="card"><h2>Printers</h2><div id="printer-list">Loading...</div></div>';
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

    async showOAuth(el) {
        el.innerHTML = `
            <div class="card">
                <h2>OAuth Clients</h2>
                <div id="oauth-clients">Loading...</div>
            </div>
            <div class="card">
                <h2>Signing Keys</h2>
                <p>RSA-2048 key used for JWT token signing.</p>
                <button id="regen-keys" class="btn btn-danger">Regenerate Keys</button>
                <p style="margin-top:8px;color:var(--text-muted);font-size:13px">Warning: regenerating keys will invalidate all existing tokens.</p>
            </div>`;

        document.getElementById('regen-keys').addEventListener('click', async () => {
            if (!confirm('Regenerate signing keys? All existing tokens will be invalidated.')) return;
            try {
                const resp = await fetch('/admin/api/oauth/keys/regenerate', { method: 'POST' });
                if (resp.ok) this.toast('Keys regenerated', 'success');
                else this.toast('Failed', 'error');
            } catch (e) {
                this.toast('Error: ' + e.message, 'error');
            }
        });

        try {
            const resp = await fetch('/admin/api/oauth/clients');
            const clients = await resp.json();
            if (!clients || clients.length === 0) {
                document.getElementById('oauth-clients').innerHTML = '<p>No registered clients</p>';
                return;
            }
            let html = '<table><thead><tr><th>Client ID</th><th>Name</th><th>Created</th><th>Actions</th></tr></thead><tbody>';
            for (const c of clients) {
                const created = new Date(c.created_at * 1000).toLocaleDateString();
                html += `<tr>
                    <td><code>${c.client_id.substring(0, 12)}...</code></td>
                    <td>${c.client_name || '-'}</td>
                    <td>${created}</td>
                    <td><button class="btn btn-danger" onclick="App.deleteClient('${c.client_id}')">Delete</button></td>
                </tr>`;
            }
            html += '</tbody></table>';
            document.getElementById('oauth-clients').innerHTML = html;
        } catch (e) {
            document.getElementById('oauth-clients').innerHTML = '<p class="error">Failed to load clients</p>';
        }
    },

    async deleteClient(clientId) {
        if (!confirm('Delete this OAuth client?')) return;
        try {
            const resp = await fetch(`/admin/api/oauth/clients/${clientId}`, { method: 'DELETE' });
            if (resp.ok) {
                this.toast('Client deleted', 'success');
                this.showOAuth(document.querySelector('.main'));
            } else {
                this.toast('Failed', 'error');
            }
        } catch (e) {
            this.toast('Error: ' + e.message, 'error');
        }
    },

    async showLogs(el) {
        el.innerHTML = '<div class="card"><h2>Server Logs</h2><div id="log-content" class="log-viewer">Loading...</div></div>';
        try {
            const resp = await fetch('/admin/api/logs');
            const text = await resp.text();
            document.getElementById('log-content').textContent = text || 'No logs available';
        } catch (e) {
            document.getElementById('log-content').textContent = 'Failed to load logs';
        }
    },

    async showStatus(el) {
        el.innerHTML = '<div class="card"><h2>Server Status</h2><div id="status-info">Loading...</div></div>';
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
