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
            case 'dns': this.showDNS(main); break;
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
            const cert = status.certificate || {};
            delete status.certificate;

            let html = '<table>';
            for (const [key, value] of Object.entries(status)) {
                html += `<tr><td><strong>${key}</strong></td><td>${value}</td></tr>`;
            }
            html += '</table>';

            html += '<h3 style="margin-top:20px">TLS Certificate</h3><table>';
            html += `<tr><td><strong>Mode</strong></td><td><span class="badge ${cert.mode === 'acme' ? 'badge-success' : 'badge-warning'}">${cert.mode || 'unknown'}</span></td></tr>`;
            html += `<tr><td><strong>Domain</strong></td><td>${cert.domain || '-'}</td></tr>`;
            html += `<tr><td><strong>Issuer</strong></td><td>${cert.issuer || '-'}</td></tr>`;
            if (cert.notBefore) html += `<tr><td><strong>Valid From</strong></td><td>${new Date(cert.notBefore).toLocaleString()}</td></tr>`;
            if (cert.notAfter) html += `<tr><td><strong>Valid Until</strong></td><td>${new Date(cert.notAfter).toLocaleString()}</td></tr>`;
            html += '</table>';

            document.getElementById('status-info').innerHTML = html;
        } catch (e) {
            document.getElementById('status-info').innerHTML = '<p class="error">Failed to load status</p>';
        }
    },

    async showDNS(el) {
        el.innerHTML = `
            <div class="card">
                <h2>DNS / Route 53 Configuration</h2>
                <p style="margin-bottom:16px;color:var(--text-muted)">Automatically update an AWS Route 53 A record with this machine's public IP address.</p>
                <div id="dns-form">Loading...</div>
            </div>
            <div class="card">
                <h2>Required IAM Policy</h2>
                <p style="margin-bottom:12px;color:var(--text-muted)">Create an IAM user with this policy and enter the credentials above.</p>
                <pre id="iam-policy" class="log-viewer" style="max-height:300px;font-size:12px">Loading...</pre>
            </div>
            <div class="card">
                <h2>DNS Update Status</h2>
                <div id="dns-status">Loading...</div>
            </div>`;

        try {
            const [cfgResp, statusResp, policyResp] = await Promise.all([
                fetch('/admin/api/dns/config'),
                fetch('/admin/api/dns/status'),
                fetch('/admin/api/dns/policy'),
            ]);
            const cfg = await cfgResp.json();
            const status = await statusResp.json();
            const policy = await policyResp.json();

            document.getElementById('dns-form').innerHTML = this.renderDNSForm(cfg);
            document.getElementById('iam-policy').textContent = policy.policy || 'Unable to load policy';
            document.getElementById('dns-status').innerHTML = this.renderDNSStatus(status);

            document.getElementById('save-dns').addEventListener('click', () => this.saveDNSConfig());
            document.getElementById('test-dns').addEventListener('click', () => this.testDNS());
        } catch (e) {
            document.getElementById('dns-form').innerHTML = '<p class="error">Failed to load DNS config</p>';
        }
    },

    renderDNSForm(cfg) {
        return `
            <div class="form-group">
                <label><input type="checkbox" id="dns-enabled" ${cfg.dnsEnabled ? 'checked' : ''}> Enable automatic DNS updates</label>
            </div>
            <div class="form-group">
                <label>Full Domain Name</label>
                <input type="text" id="dns-domain" value="${cfg.dnsDomain || ''}" placeholder="printer.example.com">
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>AWS Access Key ID</label>
                    <input type="text" id="dns-accessKey" value="${cfg.awsAccessKeyId || ''}" placeholder="AKIAIOSFODNN7EXAMPLE">
                </div>
                <div class="form-group">
                    <label>AWS Secret Access Key</label>
                    <input type="password" id="dns-secretKey" value="" placeholder="${cfg.hasSecretKey ? '(saved - enter new to change)' : 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'}">
                </div>
            </div>
            <div class="form-group">
                <label>Update Interval (seconds)</label>
                <input type="number" id="dns-interval" value="${cfg.dnsUpdateInterval || 300}" min="60" placeholder="300">
                <span style="font-size:12px;color:var(--text-muted)">Minimum 60 seconds. Default 300 (5 minutes).</span>
            </div>
            <div style="display:flex;gap:12px;margin-top:8px">
                <button id="save-dns" class="btn btn-primary">Save DNS Configuration</button>
                <button id="test-dns" class="btn btn-secondary">Test Update Now</button>
            </div>
        `;
    },

    renderDNSStatus(status) {
        const updater = status.updater || {};
        const cfg = status.config || {};
        let html = '<table>';
        html += `<tr><td><strong>Enabled</strong></td><td><span class="badge ${cfg.enabled ? 'badge-success' : 'badge-warning'}">${cfg.enabled ? 'Yes' : 'No'}</span></td></tr>`;
        html += `<tr><td><strong>Current Public IP</strong></td><td>${status.currentPublicIp || 'Unknown'}</td></tr>`;
        if (updater.domain) html += `<tr><td><strong>DNS Domain</strong></td><td>${updater.domain}</td></tr>`;
        if (updater.hostedZoneId) html += `<tr><td><strong>Hosted Zone ID</strong></td><td>${updater.hostedZoneId}</td></tr>`;
        if (updater.publicIp) html += `<tr><td><strong>Last Updated IP</strong></td><td>${updater.publicIp}</td></tr>`;
        if (updater.lastUpdate) html += `<tr><td><strong>Last Update</strong></td><td>${new Date(updater.lastUpdate).toLocaleString()}</td></tr>`;
        if (updater.nextUpdate) html += `<tr><td><strong>Next Update</strong></td><td>${new Date(updater.nextUpdate).toLocaleString()}</td></tr>`;
        html += `<tr><td><strong>Update Count</strong></td><td>${updater.updateCount || 0}</td></tr>`;
        if (updater.lastError) html += `<tr><td><strong>Last Error</strong></td><td style="color:var(--danger)">${updater.lastError}</td></tr>`;
        html += '</table>';
        return html;
    },

    async saveDNSConfig() {
        const cfg = {
            dnsEnabled: document.getElementById('dns-enabled').checked,
            dnsDomain: document.getElementById('dns-domain').value,
            awsAccessKeyId: document.getElementById('dns-accessKey').value,
            awsSecretAccessKey: document.getElementById('dns-secretKey').value,
            dnsUpdateInterval: parseInt(document.getElementById('dns-interval').value) || 300,
        };

        try {
            const resp = await fetch('/admin/api/dns/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(cfg),
            });
            const result = await resp.json();
            if (resp.ok) {
                this.toast(result.error || 'DNS configuration saved', result.error ? 'error' : 'success');
                this.showDNS(document.querySelector('.main'));
            } else {
                this.toast('Failed to save DNS config', 'error');
            }
        } catch (e) {
            this.toast('Error: ' + e.message, 'error');
        }
    },

    async testDNS() {
        this.toast('Testing DNS update...', 'success');
        try {
            const resp = await fetch('/admin/api/dns/test', { method: 'POST' });
            const result = await resp.json();
            if (resp.ok) {
                this.toast(result.result || 'DNS update successful', 'success');
                this.showDNS(document.querySelector('.main'));
            } else {
                this.toast('DNS test failed: ' + (result.message || resp.statusText), 'error');
            }
        } catch (e) {
            this.toast('Error: ' + e.message, 'error');
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
