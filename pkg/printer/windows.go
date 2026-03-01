package printer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// runPS executes a PowerShell command and returns the output.
func runPS(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("powershell error: %w\noutput: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// ListPrinters returns all installed printers.
func ListPrinters() ([]PrinterInfo, error) {
	script := `Get-Printer | Select-Object Name, DriverName, PortName, Shared, ShareName, Location, Comment, PrinterStatus, Type | ConvertTo-Json -Depth 3`
	out, err := runPS(script)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []PrinterInfo{}, nil
	}

	// Get default printer
	defaultName, _ := GetDefaultPrinter()

	var raw interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse printer list: %w", err)
	}

	var printers []PrinterInfo
	switch v := raw.(type) {
	case map[string]interface{}:
		p := parsePrinterInfo(v, defaultName)
		printers = append(printers, p)
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				p := parsePrinterInfo(m, defaultName)
				printers = append(printers, p)
			}
		}
	}

	return printers, nil
}

func parsePrinterInfo(m map[string]interface{}, defaultName string) PrinterInfo {
	name := jsonStr(m, "Name")
	p := PrinterInfo{
		Name:         name,
		DriverName:   jsonStr(m, "DriverName"),
		PortName:     jsonStr(m, "PortName"),
		Shared:       jsonBool(m, "Shared"),
		ShareName:    jsonStr(m, "ShareName"),
		Location:     jsonStr(m, "Location"),
		Comment:      jsonStr(m, "Comment"),
		PrinterState: parsePrinterStatus(m["PrinterStatus"]),
		IsDefault:    name == defaultName,
		Type:         parsePrinterType(m["Type"]),
	}
	return p
}

func parsePrinterStatus(v interface{}) string {
	switch n := v.(type) {
	case float64:
		switch int(n) {
		case 0:
			return "Normal"
		case 1:
			return "Paused"
		case 2:
			return "Error"
		case 3:
			return "PendingDeletion"
		case 4:
			return "PaperJam"
		case 5:
			return "PaperOut"
		case 6:
			return "ManualFeed"
		case 7:
			return "PaperProblem"
		case 8:
			return "Offline"
		default:
			return fmt.Sprintf("Unknown(%d)", int(n))
		}
	case string:
		return n
	}
	return "Unknown"
}

func parsePrinterType(v interface{}) string {
	switch n := v.(type) {
	case float64:
		switch int(n) {
		case 0:
			return "local"
		case 1:
			return "network"
		}
	case string:
		return n
	}
	return "local"
}

// GetPrinterDetails returns detailed info including capabilities.
func GetPrinterDetails(name string) (*PrinterInfo, error) {
	// Get basic info
	script := fmt.Sprintf(`Get-Printer -Name '%s' | Select-Object Name, DriverName, PortName, Shared, ShareName, Location, Comment, PrinterStatus, Type | ConvertTo-Json`, escapePSString(name))
	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("get printer %q: %w", name, err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("parse printer: %w", err)
	}

	defaultName, _ := GetDefaultPrinter()
	p := parsePrinterInfo(m, defaultName)

	// Get capabilities
	caps, err := getCapabilities(name)
	if err == nil {
		p.Capabilities = caps
	}

	return &p, nil
}

func getCapabilities(name string) (*Capabilities, error) {
	script := fmt.Sprintf(`
$config = Get-PrintConfiguration -PrinterName '%s' -ErrorAction SilentlyContinue
$caps = Get-PrinterProperty -PrinterName '%s' -ErrorAction SilentlyContinue
$result = @{
    Color = if ($config) { $config.Color } else { $false }
    Duplex = if ($config) { $config.DuplexingMode -ne 'OneSided' } else { $false }
    Collate = if ($config) { $config.Collate } else { $false }
}
$result | ConvertTo-Json`, escapePSString(name), escapePSString(name))

	out, err := runPS(script)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return &Capabilities{}, nil
	}

	return &Capabilities{
		Color:   jsonBool(m, "Color"),
		Duplex:  jsonBool(m, "Duplex"),
		Collate: jsonBool(m, "Collate"),
	}, nil
}

// GetDefaultPrinter returns the name of the default printer.
func GetDefaultPrinter() (string, error) {
	script := `(Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Default -eq $true}).Name`
	out, err := runPS(script)
	if err != nil {
		return "", err
	}
	return out, nil
}

// SetDefaultPrinter sets the default printer.
func SetDefaultPrinter(name string) error {
	script := fmt.Sprintf(`
$printer = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
if ($printer) {
    Invoke-CimMethod -InputObject $printer -MethodName SetDefaultPrinter | Out-Null
    Write-Output 'OK'
} else {
    Write-Error "Printer not found: %s"
}`, escapePSString(name), escapePSString(name))
	out, err := runPS(script)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("failed to set default printer: %s", out)
	}
	return nil
}

// PrintFile sends a file to a printer.
func PrintFile(filePath string, opts PrintOptions) error {
	printerName := opts.PrinterName
	if printerName == "" {
		var err error
		printerName, err = GetDefaultPrinter()
		if err != nil {
			return fmt.Errorf("no printer specified and no default printer: %w", err)
		}
	}

	copies := opts.Copies
	if copies <= 0 {
		copies = 1
	}

	script := fmt.Sprintf(`Start-Process -FilePath '%s' -Verb Print -ArgumentList '/d:"%s"' -Wait -ErrorAction Stop`,
		escapePSString(filePath), escapePSString(printerName))

	// For known file types use Out-Printer
	ext := strings.ToLower(filePath)
	if strings.HasSuffix(ext, ".txt") || strings.HasSuffix(ext, ".log") || strings.HasSuffix(ext, ".csv") {
		script = fmt.Sprintf(`Get-Content '%s' | Out-Printer -Name '%s'`,
			escapePSString(filePath), escapePSString(printerName))
	}

	_, err := runPS(script)
	return err
}

// PrintText sends raw text to a printer.
func PrintText(text string, opts PrintOptions) error {
	printerName := opts.PrinterName
	if printerName == "" {
		var err error
		printerName, err = GetDefaultPrinter()
		if err != nil {
			return fmt.Errorf("no printer specified and no default printer: %w", err)
		}
	}

	// Use a temp file approach for reliability
	script := fmt.Sprintf(`
$text = @'
%s
'@
$text | Out-Printer -Name '%s'`, text, escapePSString(printerName))

	_, err := runPS(script)
	return err
}

// PrintImage sends an image to a printer with photo-optimized settings.
func PrintImage(imagePath string, opts PrintOptions) error {
	printerName := opts.PrinterName
	if printerName == "" {
		var err error
		printerName, err = GetDefaultPrinter()
		if err != nil {
			return fmt.Errorf("no printer specified and no default printer: %w", err)
		}
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Drawing
$img = [System.Drawing.Image]::FromFile('%s')
$doc = New-Object System.Drawing.Printing.PrintDocument
$doc.PrinterSettings.PrinterName = '%s'
$doc.PrinterSettings.Copies = %d
$doc.PrintPage.Add({
    param($sender, $e)
    $destRect = $e.MarginBounds
    $e.Graphics.DrawImage($img, $destRect)
})
$doc.Print()
$img.Dispose()`, escapePSString(imagePath), escapePSString(printerName), max(opts.Copies, 1))

	_, err := runPS(script)
	return err
}

// GetPrintQueue returns jobs in the print queue.
func GetPrintQueue(printerName string) ([]PrintJob, error) {
	var script string
	if printerName != "" {
		script = fmt.Sprintf(`Get-PrintJob -PrinterName '%s' | Select-Object Id, DocumentName, UserName, PrinterName, JobStatus, Priority, Size, SubmittedTime, TotalPages, PagesPrinted | ConvertTo-Json -Depth 3`, escapePSString(printerName))
	} else {
		script = `Get-Printer | ForEach-Object { Get-PrintJob -PrinterName $_.Name -ErrorAction SilentlyContinue } | Select-Object Id, DocumentName, UserName, PrinterName, JobStatus, Priority, Size, SubmittedTime, TotalPages, PagesPrinted | ConvertTo-Json -Depth 3`
	}

	out, err := runPS(script)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []PrintJob{}, nil
	}

	var raw interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse print queue: %w", err)
	}

	var jobs []PrintJob
	switch v := raw.(type) {
	case map[string]interface{}:
		jobs = append(jobs, parsePrintJob(v))
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				jobs = append(jobs, parsePrintJob(m))
			}
		}
	}

	return jobs, nil
}

func parsePrintJob(m map[string]interface{}) PrintJob {
	return PrintJob{
		JobID:        jsonInt(m, "Id"),
		Document:     jsonStr(m, "DocumentName"),
		Owner:        jsonStr(m, "UserName"),
		PrinterName:  jsonStr(m, "PrinterName"),
		Status:       jsonStr(m, "JobStatus"),
		Priority:     jsonInt(m, "Priority"),
		Size:         int64(jsonFloat(m, "Size")),
		SubmittedAt:  jsonStr(m, "SubmittedTime"),
		Pages:        jsonInt(m, "TotalPages"),
		PagesPrinted: jsonInt(m, "PagesPrinted"),
	}
}

// GetPrintJobStatus returns the status of a specific print job.
func GetPrintJobStatus(printerName string, jobID int) (*PrintJob, error) {
	script := fmt.Sprintf(`Get-PrintJob -PrinterName '%s' -ID %d | Select-Object Id, DocumentName, UserName, PrinterName, JobStatus, Priority, Size, SubmittedTime, TotalPages, PagesPrinted | ConvertTo-Json`, escapePSString(printerName), jobID)
	out, err := runPS(script)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("parse job status: %w", err)
	}

	job := parsePrintJob(m)
	return &job, nil
}

// CancelPrintJob cancels a print job.
func CancelPrintJob(printerName string, jobID int) error {
	script := fmt.Sprintf(`Remove-PrintJob -PrinterName '%s' -ID %d`, escapePSString(printerName), jobID)
	_, err := runPS(script)
	return err
}

// PausePrinter pauses a printer.
func PausePrinter(name string) error {
	script := fmt.Sprintf(`
$printer = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
if ($printer) {
    Invoke-CimMethod -InputObject $printer -MethodName Pause | Out-Null
    Write-Output 'OK'
} else {
    Write-Error "Printer not found: %s"
}`, escapePSString(name), escapePSString(name))
	out, err := runPS(script)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("failed to pause printer: %s", out)
	}
	return nil
}

// ResumePrinter resumes a paused printer.
func ResumePrinter(name string) error {
	script := fmt.Sprintf(`
$printer = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
if ($printer) {
    Invoke-CimMethod -InputObject $printer -MethodName Resume | Out-Null
    Write-Output 'OK'
} else {
    Write-Error "Printer not found: %s"
}`, escapePSString(name), escapePSString(name))
	out, err := runPS(script)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("failed to resume printer: %s", out)
	}
	return nil
}

// PrintTestPage prints a Windows test page.
func PrintTestPage(name string) error {
	script := fmt.Sprintf(`
$printer = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
if ($printer) {
    Invoke-CimMethod -InputObject $printer -MethodName PrintTestPage | Out-Null
    Write-Output 'OK'
} else {
    Write-Error "Printer not found: %s"
}`, escapePSString(name), escapePSString(name))
	out, err := runPS(script)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("failed to print test page: %s", out)
	}
	return nil
}

// GetPaperSizes returns the supported paper sizes for a printer.
func GetPaperSizes(name string) ([]PaperSize, error) {
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Drawing
$doc = New-Object System.Drawing.Printing.PrintDocument
$doc.PrinterSettings.PrinterName = '%s'
if (-not $doc.PrinterSettings.IsValid) {
    Write-Error "Invalid printer: %s"
    return
}
$sizes = @()
foreach ($s in $doc.PrinterSettings.PaperSizes) {
    $sizes += [PSCustomObject]@{
        Name = $s.PaperName
        Width = $s.Width
        Height = $s.Height
    }
}
$doc.Dispose()
$sizes | ConvertTo-Json -Depth 2`, escapePSString(name), escapePSString(name))

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("get paper sizes for %q: %w", name, err)
	}
	if out == "" {
		return []PaperSize{}, nil
	}

	var raw interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse paper sizes: %w", err)
	}

	var sizes []PaperSize
	parsePaper := func(m map[string]interface{}) PaperSize {
		// Width/Height from System.Drawing are in hundredths of an inch
		w := jsonFloat(m, "Width")
		h := jsonFloat(m, "Height")
		return PaperSize{
			Name:     jsonStr(m, "Name"),
			WidthIn:  roundTo(w/100.0, 2),
			HeightIn: roundTo(h/100.0, 2),
			WidthMM:  roundTo(w*0.254, 1),
			HeightMM: roundTo(h*0.254, 1),
		}
	}

	switch v := raw.(type) {
	case map[string]interface{}:
		sizes = append(sizes, parsePaper(v))
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				sizes = append(sizes, parsePaper(m))
			}
		}
	}

	return sizes, nil
}

// ListPrintersWithPaperSizes returns all printers with their supported paper sizes.
func ListPrintersWithPaperSizes() ([]PrinterWithPaperSizes, error) {
	printers, err := ListPrinters()
	if err != nil {
		return nil, err
	}

	var result []PrinterWithPaperSizes
	for _, p := range printers {
		sizes, err := GetPaperSizes(p.Name)
		if err != nil {
			sizes = []PaperSize{} // include printer even if paper size query fails
		}
		result = append(result, PrinterWithPaperSizes{
			Name:       p.Name,
			DriverName: p.DriverName,
			IsDefault:  p.IsDefault,
			PaperSizes: sizes,
		})
	}

	return result, nil
}

// PrintAllTestPages sends a Windows test page to every installed printer.
// Returns a map of printer name to result (success message or error).
func PrintAllTestPages() map[string]string {
	printers, err := ListPrinters()
	if err != nil {
		return map[string]string{"error": fmt.Sprintf("Failed to list printers: %s", err)}
	}

	results := make(map[string]string)
	for _, p := range printers {
		if err := PrintTestPage(p.Name); err != nil {
			results[p.Name] = fmt.Sprintf("error: %s", err)
		} else {
			results[p.Name] = "test page sent"
		}
	}
	return results
}

func roundTo(val float64, places int) float64 {
	pow := 1.0
	for i := 0; i < places; i++ {
		pow *= 10
	}
	return float64(int(val*pow+0.5)) / pow
}

// escapePSString escapes single quotes in PowerShell strings.
func escapePSString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// JSON helper functions
func jsonStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func jsonBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func jsonInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case string:
			i, _ := strconv.Atoi(n)
			return i
		}
	}
	return 0
}

func jsonFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// parseDetectedErrorState maps WMI DetectedErrorState codes to descriptions.
func parseDetectedErrorState(code int) string {
	switch code {
	case 0:
		return "Unknown"
	case 1:
		return "Other"
	case 2:
		return "No Error"
	case 3:
		return "Low Paper"
	case 4:
		return "No Paper"
	case 5:
		return "Low Toner"
	case 6:
		return "No Toner"
	case 7:
		return "Door Open"
	case 8:
		return "Jammed"
	case 9:
		return "Offline"
	case 10:
		return "Service Requested"
	case 11:
		return "Output Bin Full"
	default:
		return fmt.Sprintf("Unknown(%d)", code)
	}
}

// parseExtendedErrorState maps WMI ExtendedDetectedErrorState codes to descriptions.
func parseExtendedErrorState(code int) string {
	switch code {
	case 0:
		return "Unknown"
	case 1:
		return "Other"
	case 2:
		return "No Error"
	case 3:
		return "Low Paper"
	case 4:
		return "No Paper"
	case 5:
		return "Low Toner"
	case 6:
		return "No Toner"
	case 7:
		return "Door Open"
	case 8:
		return "Jammed"
	case 9:
		return "Offline"
	case 10:
		return "Service Requested"
	case 11:
		return "Output Bin Full"
	case 12:
		return "Paper Problem"
	case 13:
		return "Cannot Print Page"
	case 14:
		return "User Intervention Required"
	case 15:
		return "Out of Memory"
	case 16:
		return "Server Unknown"
	default:
		return fmt.Sprintf("Unknown(%d)", code)
	}
}

// extractIPFromPort extracts an IP address from a printer port name like "IP_192.168.1.100".
func extractIPFromPort(portName string) string {
	portName = strings.TrimSpace(portName)
	if strings.HasPrefix(portName, "IP_") {
		return strings.TrimPrefix(portName, "IP_")
	}
	if strings.HasPrefix(portName, "TCPIP_") {
		return strings.TrimPrefix(portName, "TCPIP_")
	}
	// Check if it looks like an IP address directly
	parts := strings.Split(portName, ".")
	if len(parts) == 4 {
		return portName
	}
	return ""
}

// GetInkTonerLevels returns ink/toner supply levels for a printer.
func GetInkTonerLevels(name string) (*InkTonerStatus, error) {
	script := fmt.Sprintf(`
$printer = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
if (-not $printer) { Write-Error "Printer not found: %s"; return }
$result = @{
    PrinterName = $printer.Name
    Status = $printer.PrinterStatus
    ErrorState = $printer.DetectedErrorState
    SNMPAvailable = $false
    Supplies = @()
}
# Try SNMP for network printers
$port = (Get-PrinterPort -Name $printer.PortName -ErrorAction SilentlyContinue)
if ($port -and $port.PrinterHostAddress) {
    try {
        $snmp = New-Object -ComObject olePrn.OleSNMP
        $snmp.Open($port.PrinterHostAddress, "public", 2, 3000)
        $result.SNMPAvailable = $true
        # RFC 3805 OIDs: supply descriptions, max levels, current levels
        $names = $snmp.GetTree("43.11.1.1.6.1")
        $maxLevels = $snmp.GetTree("43.11.1.1.8.1")
        $currLevels = $snmp.GetTree("43.11.1.1.9.1")
        $supplies = @()
        for ($i = 0; $i -lt $names.Count; $i++) {
            $maxVal = if ($i -lt $maxLevels.Count) { [int]$maxLevels[$i] } else { 0 }
            $currVal = if ($i -lt $currLevels.Count) { [int]$currLevels[$i] } else { 0 }
            $pct = if ($maxVal -gt 0) { [math]::Round(($currVal / $maxVal) * 100, 1) } else { -1 }
            $supplies += @{
                Name = [string]$names[$i]
                Level = $pct
                MaxLevel = $maxVal
                CurrLevel = $currVal
            }
        }
        $result.Supplies = $supplies
        $snmp.Close()
    } catch {
        $result.SNMPAvailable = $false
    }
}
$result | ConvertTo-Json -Depth 3`, escapePSString(name), escapePSString(name))

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("get ink/toner levels for %q: %w", name, err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("parse ink/toner levels: %w", err)
	}

	status := &InkTonerStatus{
		PrinterName:   jsonStr(m, "PrinterName"),
		Status:        fmt.Sprintf("%v", m["Status"]),
		ErrorState:    parseDetectedErrorState(jsonInt(m, "ErrorState")),
		SNMPAvailable: jsonBool(m, "SNMPAvailable"),
	}

	if supplies, ok := m["Supplies"]; ok {
		switch v := supplies.(type) {
		case []interface{}:
			for _, item := range v {
				if sm, ok := item.(map[string]interface{}); ok {
					status.Supplies = append(status.Supplies, InkTonerLevel{
						Name:      jsonStr(sm, "Name"),
						Level:     jsonFloat(sm, "Level"),
						MaxLevel:  jsonInt(sm, "MaxLevel"),
						CurrLevel: jsonInt(sm, "CurrLevel"),
					})
				}
			}
		}
	}

	return status, nil
}

// GetPrintHistory returns recent print history from the Windows event log.
func GetPrintHistory(days int, printerName string) ([]PrintHistoryEntry, error) {
	if days <= 0 {
		days = 7
	}
	filterClause := ""
	if printerName != "" {
		filterClause = fmt.Sprintf(` | Where-Object { $_.Properties[4].Value -eq '%s' }`, escapePSString(printerName))
	}

	script := fmt.Sprintf(`
try {
    $log = Get-WinEvent -LogName 'Microsoft-Windows-PrintService/Operational' -FilterXPath "*[System[(EventID=307) and TimeCreated[timediff(@SystemTime) <= %d]]]" -ErrorAction Stop%s |
        Select-Object -First 100 |
        ForEach-Object {
            @{
                JobID = [int]$_.Properties[0].Value
                Document = [string]$_.Properties[1].Value
                User = [string]$_.Properties[2].Value
                PrinterName = [string]$_.Properties[4].Value
                Pages = [int]$_.Properties[7].Value
                Size = [int64]$_.Properties[6].Value
                Timestamp = $_.TimeCreated.ToString('o')
            }
        }
    if ($log) { $log | ConvertTo-Json -Depth 2 } else { '[]' }
} catch {
    if ($_.Exception.Message -match 'No events were found') { '[]' }
    elseif ($_.Exception.Message -match 'could not be found') {
        Write-Error "Print history log is not enabled. Enable it via: wevtutil sl Microsoft-Windows-PrintService/Operational /e:true"
    } else { throw }
}`, days*86400000, filterClause)

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("get print history: %w", err)
	}
	if out == "" || out == "[]" {
		return []PrintHistoryEntry{}, nil
	}

	var raw interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse print history: %w", err)
	}

	var entries []PrintHistoryEntry
	parseEntry := func(m map[string]interface{}) PrintHistoryEntry {
		return PrintHistoryEntry{
			JobID:       jsonInt(m, "JobID"),
			Document:    jsonStr(m, "Document"),
			User:        jsonStr(m, "User"),
			PrinterName: jsonStr(m, "PrinterName"),
			Pages:       jsonInt(m, "Pages"),
			Size:        int64(jsonFloat(m, "Size")),
			Timestamp:   jsonStr(m, "Timestamp"),
		}
	}

	switch v := raw.(type) {
	case map[string]interface{}:
		entries = append(entries, parseEntry(v))
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				entries = append(entries, parseEntry(m))
			}
		}
	}

	return entries, nil
}

// TestPrinterConnectivity tests whether a printer is reachable.
func TestPrinterConnectivity(name string) (*ConnectivityResult, error) {
	script := fmt.Sprintf(`
$printer = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
$result = @{
    PrinterName = '%s'
    Exists = $false
    WMIStatus = ''
    ErrorState = 0
    ErrorStateDesc = ''
    IsNetwork = $false
    PingSuccess = $false
    Port9100Open = $false
}
if ($printer) {
    $result.Exists = $true
    $result.WMIStatus = [string]$printer.PrinterStatus
    $result.ErrorState = [int]$printer.DetectedErrorState
    $result.IsNetwork = ($printer.Network -eq $true) -or ($printer.PortName -match 'IP_|TCPIP_')
    $portName = $printer.PortName
    $ip = ''
    if ($portName -match 'IP_(.+)') { $ip = $Matches[1] }
    elseif ($portName -match 'TCPIP_(.+)') { $ip = $Matches[1] }
    else {
        $port = Get-PrinterPort -Name $portName -ErrorAction SilentlyContinue
        if ($port -and $port.PrinterHostAddress) { $ip = $port.PrinterHostAddress }
    }
    if ($ip) {
        $result.PingSuccess = (Test-Connection -ComputerName $ip -Count 1 -Quiet -ErrorAction SilentlyContinue)
        try {
            $tcp = New-Object System.Net.Sockets.TcpClient
            $task = $tcp.ConnectAsync($ip, 9100)
            $result.Port9100Open = $task.Wait(2000)
            $tcp.Close()
        } catch { $result.Port9100Open = $false }
    }
}
$result | ConvertTo-Json`, escapePSString(name), escapePSString(name))

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("test connectivity for %q: %w", name, err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("parse connectivity result: %w", err)
	}

	errState := jsonInt(m, "ErrorState")
	return &ConnectivityResult{
		PrinterName:    jsonStr(m, "PrinterName"),
		Exists:         jsonBool(m, "Exists"),
		WMIStatus:      jsonStr(m, "WMIStatus"),
		ErrorState:     fmt.Sprintf("%d", errState),
		ErrorStateDesc: parseDetectedErrorState(errState),
		IsNetwork:      jsonBool(m, "IsNetwork"),
		PingSuccess:    jsonBool(m, "PingSuccess"),
		Port9100Open:   jsonBool(m, "Port9100Open"),
	}, nil
}

// PurgePrintQueue removes all jobs from a printer's queue. Returns the count of removed jobs.
func PurgePrintQueue(name string) (int, error) {
	script := fmt.Sprintf(`
$jobs = Get-PrintJob -PrinterName '%s' -ErrorAction SilentlyContinue
$count = 0
if ($jobs) {
    $jobs | ForEach-Object {
        Remove-PrintJob -PrinterName '%s' -ID $_.Id -ErrorAction SilentlyContinue
        $count++
    }
}
Write-Output $count`, escapePSString(name), escapePSString(name))

	out, err := runPS(script)
	if err != nil {
		return 0, fmt.Errorf("purge print queue for %q: %w", name, err)
	}
	count, _ := strconv.Atoi(strings.TrimSpace(out))
	return count, nil
}

// RestartPrintJob restarts a specific print job.
func RestartPrintJob(printerName string, jobID int) error {
	script := fmt.Sprintf(`Restart-PrintJob -PrinterName '%s' -ID %d`, escapePSString(printerName), jobID)
	_, err := runPS(script)
	if err != nil {
		return fmt.Errorf("restart job %d on %q: %w", jobID, printerName, err)
	}
	return nil
}

// AddNetworkPrinter adds a network printer by UNC path or IP address.
func AddNetworkPrinter(connectionName, ipAddress, driverName, printerName string) error {
	if connectionName != "" {
		// UNC path mode: \\server\printer
		script := fmt.Sprintf(`Add-Printer -ConnectionName '%s'`, escapePSString(connectionName))
		_, err := runPS(script)
		if err != nil {
			return fmt.Errorf("add network printer %q: %w", connectionName, err)
		}
		return nil
	}

	// IP address mode
	if ipAddress == "" {
		return fmt.Errorf("either connection_name (UNC path) or ip_address must be provided")
	}
	if driverName == "" {
		return fmt.Errorf("driver_name is required when adding by IP address")
	}
	if printerName == "" {
		printerName = fmt.Sprintf("Printer_%s", ipAddress)
	}

	portName := fmt.Sprintf("IP_%s", ipAddress)
	script := fmt.Sprintf(`
$existingPort = Get-PrinterPort -Name '%s' -ErrorAction SilentlyContinue
if (-not $existingPort) {
    Add-PrinterPort -Name '%s' -PrinterHostAddress '%s'
}
Add-Printer -Name '%s' -DriverName '%s' -PortName '%s'`,
		escapePSString(portName),
		escapePSString(portName), escapePSString(ipAddress),
		escapePSString(printerName), escapePSString(driverName), escapePSString(portName))

	_, err := runPS(script)
	if err != nil {
		return fmt.Errorf("add IP printer %q at %s: %w", printerName, ipAddress, err)
	}
	return nil
}

// RemovePrinter removes an installed printer.
func RemovePrinter(name string) error {
	script := fmt.Sprintf(`Remove-Printer -Name '%s'`, escapePSString(name))
	_, err := runPS(script)
	if err != nil {
		return fmt.Errorf("remove printer %q: %w", name, err)
	}
	return nil
}

// SetPrintDefaults sets default print configuration for a printer.
func SetPrintDefaults(cfg PrintDefaultsConfig) error {
	var parts []string
	parts = append(parts, fmt.Sprintf("-PrinterName '%s'", escapePSString(cfg.PrinterName)))

	if cfg.PaperSize != "" {
		parts = append(parts, fmt.Sprintf("-PaperSize '%s'", escapePSString(cfg.PaperSize)))
	}
	if cfg.Color != nil {
		if *cfg.Color {
			parts = append(parts, "-Color $true")
		} else {
			parts = append(parts, "-Color $false")
		}
	}
	if cfg.DuplexMode != "" {
		parts = append(parts, fmt.Sprintf("-DuplexingMode '%s'", escapePSString(cfg.DuplexMode)))
	}
	if cfg.Collate != nil {
		if *cfg.Collate {
			parts = append(parts, "-Collate $true")
		} else {
			parts = append(parts, "-Collate $false")
		}
	}

	script := "Set-PrintConfiguration " + strings.Join(parts, " ")
	_, err := runPS(script)
	if err != nil {
		return fmt.Errorf("set print defaults for %q: %w", cfg.PrinterName, err)
	}
	return nil
}

// SharePrinter enables or disables printer sharing.
func SharePrinter(name string, shared bool, shareName string) error {
	sharedStr := "$false"
	if shared {
		sharedStr = "$true"
	}

	var script string
	if shared && shareName != "" {
		script = fmt.Sprintf(`Set-Printer -Name '%s' -Shared %s -ShareName '%s'`,
			escapePSString(name), sharedStr, escapePSString(shareName))
	} else {
		script = fmt.Sprintf(`Set-Printer -Name '%s' -Shared %s`,
			escapePSString(name), sharedStr)
	}

	_, err := runPS(script)
	if err != nil {
		return fmt.Errorf("share printer %q: %w", name, err)
	}
	return nil
}

// PrintHTML prints HTML content by writing to a temp file and using mshtml.dll.
func PrintHTML(html string, printerName string) error {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("mcp_print_%d.html", os.Getpid()))
	if err := os.WriteFile(tmpFile, []byte(html), 0600); err != nil {
		return fmt.Errorf("write temp HTML file: %w", err)
	}
	defer os.Remove(tmpFile)

	var script string
	if printerName != "" {
		// Temporarily set default printer, print, then restore
		script = fmt.Sprintf(`
$origDefault = (Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Default -eq $true}).Name
$targetPrinter = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
if ($targetPrinter) {
    Invoke-CimMethod -InputObject $targetPrinter -MethodName SetDefaultPrinter | Out-Null
}
$filePath = '%s'
rundll32.exe mshtml.dll,PrintHTML $filePath
Start-Sleep -Seconds 3
if ($origDefault) {
    $orig = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq $origDefault}
    if ($orig) { Invoke-CimMethod -InputObject $orig -MethodName SetDefaultPrinter | Out-Null }
}
Write-Output 'OK'`, escapePSString(printerName), escapePSString(tmpFile))
	} else {
		script = fmt.Sprintf(`
rundll32.exe mshtml.dll,PrintHTML "%s"
Start-Sleep -Seconds 2
Write-Output 'OK'`, escapePSString(tmpFile))
	}

	out, err := runPS(script)
	if err != nil {
		return fmt.Errorf("print HTML: %w", err)
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("print HTML failed: %s", out)
	}
	return nil
}

// PrintURL downloads a URL and prints it as HTML.
func PrintURL(url string, printerName string) error {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("mcp_url_%d.html", os.Getpid()))
	defer os.Remove(tmpFile)

	script := fmt.Sprintf(`
try {
    $response = Invoke-WebRequest -Uri '%s' -UseBasicParsing -TimeoutSec 30
    [System.IO.File]::WriteAllText('%s', $response.Content)
    Write-Output 'OK'
} catch {
    Write-Error "Failed to download URL: $_"
}`, escapePSString(url), escapePSString(tmpFile))

	out, err := runPS(script)
	if err != nil {
		return fmt.Errorf("download URL %q: %w", url, err)
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("download URL failed: %s", out)
	}

	// Read the downloaded content and print it
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		return fmt.Errorf("read downloaded content: %w", err)
	}

	return PrintHTML(string(content), printerName)
}

// PrintMarkdown converts Markdown to HTML and prints it.
func PrintMarkdown(markdown string, printerName string) error {
	html := ConvertMarkdownToHTML(markdown)
	return PrintHTML(html, printerName)
}

// PrintMultipleFiles prints multiple files to a printer. Max 50 files.
func PrintMultipleFiles(filePaths []string, printerName string) []MultiFilePrintResult {
	limit := 50
	if len(filePaths) > limit {
		filePaths = filePaths[:limit]
	}

	if printerName == "" {
		defaultName, err := GetDefaultPrinter()
		if err != nil {
			results := make([]MultiFilePrintResult, len(filePaths))
			for i, fp := range filePaths {
				results[i] = MultiFilePrintResult{FilePath: fp, Status: "error", Error: "no printer specified and no default printer"}
			}
			return results
		}
		printerName = defaultName
	}

	results := make([]MultiFilePrintResult, len(filePaths))
	for i, fp := range filePaths {
		script := fmt.Sprintf(`Start-Process -FilePath '%s' -Verb PrintTo -ArgumentList '%s' -ErrorAction Stop`,
			escapePSString(fp), escapePSString(printerName))
		_, err := runPS(script)
		if err != nil {
			results[i] = MultiFilePrintResult{FilePath: fp, Status: "error", Error: err.Error()}
		} else {
			results[i] = MultiFilePrintResult{FilePath: fp, Status: "sent"}
		}
	}
	return results
}

// GetPrinterErrors returns error information for a printer.
func GetPrinterErrors(name string) (*PrinterError, error) {
	script := fmt.Sprintf(`
$printer = Get-CimInstance -ClassName Win32_Printer | Where-Object {$_.Name -eq '%s'}
if (-not $printer) { Write-Error "Printer not found: %s"; return }
$result = @{
    PrinterName = $printer.Name
    ErrorState = [int]$printer.DetectedErrorState
    ExtendedErrorState = [int]$printer.ExtendedDetectedErrorState
    RecentErrors = @()
}
try {
    $errors = Get-WinEvent -LogName 'Microsoft-Windows-PrintService/Operational' -FilterXPath "*[System[(EventID=311) and TimeCreated[timediff(@SystemTime) <= 86400000]]]" -MaxEvents 10 -ErrorAction SilentlyContinue |
        Where-Object { $_.Message -match '%s' } |
        ForEach-Object { $_.TimeCreated.ToString('o') + ': ' + $_.Message }
    if ($errors) { $result.RecentErrors = @($errors) }
} catch {}
$result | ConvertTo-Json -Depth 2`, escapePSString(name), escapePSString(name), escapePSString(name))

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("get printer errors for %q: %w", name, err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("parse printer errors: %w", err)
	}

	errState := jsonInt(m, "ErrorState")
	extErrState := jsonInt(m, "ExtendedErrorState")

	result := &PrinterError{
		PrinterName:       jsonStr(m, "PrinterName"),
		ErrorState:        errState,
		ErrorStateDesc:    parseDetectedErrorState(errState),
		ExtendedError:     extErrState,
		ExtendedErrorDesc: parseExtendedErrorState(extErrState),
	}

	if recent, ok := m["RecentErrors"]; ok {
		switch v := recent.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					result.RecentErrors = append(result.RecentErrors, s)
				}
			}
		}
	}

	return result, nil
}
