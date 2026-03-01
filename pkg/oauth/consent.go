package oauth

import (
	"html/template"
	"net/http"
)

var consentTemplate = template.Must(template.New("consent").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Authorize Application</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
        .card { background: white; border-radius: 12px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); padding: 40px; max-width: 420px; width: 100%; }
        h1 { font-size: 24px; margin-bottom: 8px; color: #1a1a1a; }
        .app-name { color: #0066cc; font-weight: 600; }
        p { color: #666; margin: 16px 0; line-height: 1.5; }
        .scope { background: #f0f7ff; border: 1px solid #cce0ff; border-radius: 6px; padding: 12px 16px; margin: 16px 0; font-family: monospace; color: #0066cc; }
        .buttons { display: flex; gap: 12px; margin-top: 24px; }
        button { flex: 1; padding: 12px; border: none; border-radius: 8px; font-size: 16px; cursor: pointer; font-weight: 500; }
        .approve { background: #0066cc; color: white; }
        .approve:hover { background: #0052a3; }
        .deny { background: #e8e8e8; color: #333; }
        .deny:hover { background: #d0d0d0; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Authorize Application</h1>
        <p><span class="app-name">{{.ClientName}}</span> wants to access your printer server.</p>
        <div class="scope">Scope: {{.Scope}}</div>
        <p>This will allow the application to list printers and manage print jobs.</p>
        <form method="POST" action="/authorize">
            <input type="hidden" name="client_id" value="{{.ClientID}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="hidden" name="scope" value="{{.Scope}}">
            <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
            <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
            <input type="hidden" name="response_type" value="code">
            <div class="buttons">
                <button type="submit" name="action" value="deny" class="deny">Deny</button>
                <button type="submit" name="action" value="approve" class="approve">Approve</button>
            </div>
        </form>
    </div>
</body>
</html>`))

// ConsentData holds data for the consent page template.
type ConsentData struct {
	ClientID            string
	ClientName          string
	RedirectURI         string
	State               string
	Scope               string
	CodeChallenge       string
	CodeChallengeMethod string
}

// RenderConsent renders the OAuth consent page.
func RenderConsent(w http.ResponseWriter, data ConsentData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	consentTemplate.Execute(w, data)
}
