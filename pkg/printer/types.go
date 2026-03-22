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
	Type         string       `json:"type,omitempty"`     // local, network
	Category     string       `json:"category,omitempty"` // photo
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

// PaperSize describes a supported paper size for a printer.
type PaperSize struct {
	Name     string  `json:"name"`
	WidthMM  float64 `json:"widthMm"`
	HeightMM float64 `json:"heightMm"`
	WidthIn  float64 `json:"widthIn"`
	HeightIn float64 `json:"heightIn"`
}

// PrinterWithPaperSizes combines printer info with its supported paper sizes.
type PrinterWithPaperSizes struct {
	Name       string      `json:"name"`
	DriverName string      `json:"driverName,omitempty"`
	IsDefault  bool        `json:"isDefault"`
	PaperSizes []PaperSize `json:"paperSizes"`
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

// InkTonerLevel describes a single supply level for a printer.
type InkTonerLevel struct {
	Name      string  `json:"name"`
	Level     float64 `json:"level"`     // percentage 0-100
	MaxLevel  int     `json:"maxLevel"`  // raw max capacity
	CurrLevel int     `json:"currLevel"` // raw current level
	ColorCode string  `json:"colorCode,omitempty"`
}

// InkTonerStatus describes the supply status for a printer.
type InkTonerStatus struct {
	PrinterName   string          `json:"printerName"`
	Status        string          `json:"status"`
	ErrorState    string          `json:"errorState"`
	Supplies      []InkTonerLevel `json:"supplies,omitempty"`
	SNMPAvailable bool            `json:"snmpAvailable"`
}

// PrintHistoryEntry represents a completed print job from the event log.
type PrintHistoryEntry struct {
	JobID       int    `json:"jobId"`
	Document    string `json:"document"`
	User        string `json:"user"`
	PrinterName string `json:"printerName"`
	Pages       int    `json:"pages"`
	Size        int64  `json:"size"`
	Timestamp   string `json:"timestamp"`
}

// ConnectivityResult describes the connectivity status of a printer.
type ConnectivityResult struct {
	PrinterName    string `json:"printerName"`
	Exists         bool   `json:"exists"`
	WMIStatus      string `json:"wmiStatus"`
	ErrorState     string `json:"errorState"`
	ErrorStateDesc string `json:"errorStateDesc"`
	IsNetwork      bool   `json:"isNetwork"`
	PingSuccess    bool   `json:"pingSuccess"`
	Port9100Open   bool   `json:"port9100Open"`
}

// PrinterError describes error information for a printer.
type PrinterError struct {
	PrinterName       string   `json:"printerName"`
	ErrorState        int      `json:"errorState"`
	ErrorStateDesc    string   `json:"errorStateDesc"`
	ExtendedError     int      `json:"extendedErrorState"`
	ExtendedErrorDesc string   `json:"extendedErrorDesc"`
	RecentErrors      []string `json:"recentErrors,omitempty"`
}

// PrintDefaultsConfig holds print configuration defaults.
type PrintDefaultsConfig struct {
	PrinterName string `json:"printerName"`
	PaperSize   string `json:"paperSize,omitempty"`
	Color       *bool  `json:"color,omitempty"`
	DuplexMode  string `json:"duplexMode,omitempty"`
	Collate     *bool  `json:"collate,omitempty"`
}

// MultiFilePrintResult holds the result of printing a single file in a batch.
type MultiFilePrintResult struct {
	FilePath string `json:"filePath"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}
