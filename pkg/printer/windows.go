package printer

import (
	"encoding/json"
	"fmt"
	"os/exec"
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
