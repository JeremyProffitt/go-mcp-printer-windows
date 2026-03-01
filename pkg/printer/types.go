package printer

// PrinterInfo contains information about a printer.
type PrinterInfo struct {
	Name         string       `json:"name"`
	DriverName   string       `json:"driverName,omitempty"`
	PortName     string       `json:"portName,omitempty"`
	Shared       bool         `json:"shared"`
	ShareName    string       `json:"shareName,omitempty"`
	Location     string       `json:"location,omitempty"`
	Comment      string       `json:"comment,omitempty"`
	PrinterState string       `json:"printerState"`
	IsDefault    bool         `json:"isDefault"`
	Type         string       `json:"type,omitempty"` // local, network
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}

// Capabilities describes a printer's capabilities.
type Capabilities struct {
	Color       bool     `json:"color"`
	Duplex      bool     `json:"duplex"`
	Staple      bool     `json:"staple"`
	Collate     bool     `json:"collate"`
	PaperSizes  []string `json:"paperSizes,omitempty"`
	MediaTypes  []string `json:"mediaTypes,omitempty"`
	Resolutions []string `json:"resolutions,omitempty"`
}

// PrintJob represents a job in the print queue.
type PrintJob struct {
	JobID        int    `json:"jobId"`
	Document     string `json:"document"`
	Owner        string `json:"owner"`
	PrinterName  string `json:"printerName"`
	Status       string `json:"status"`
	Priority     int    `json:"priority"`
	Size         int64  `json:"size"`
	SubmittedAt  string `json:"submittedAt,omitempty"`
	Pages        int    `json:"pages,omitempty"`
	PagesPrinted int    `json:"pagesPrinted,omitempty"`
}

// PrintOptions controls how a document is printed.
type PrintOptions struct {
	PrinterName string `json:"printerName"`
	Copies      int    `json:"copies,omitempty"`
	Duplex      string `json:"duplex,omitempty"`  // None, TwoSidedLongEdge, TwoSidedShortEdge
	PaperSize   string `json:"paperSize,omitempty"`
	Orientation string `json:"orientation,omitempty"` // Portrait, Landscape
	Color       string `json:"color,omitempty"`       // Color, Grayscale
	Quality     string `json:"quality,omitempty"`      // Draft, Normal, High
	MediaType   string `json:"mediaType,omitempty"`
	FitToPage   bool   `json:"fitToPage,omitempty"`
}
