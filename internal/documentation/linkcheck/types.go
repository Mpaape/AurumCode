package linkcheck

// LinkType represents the type of link
type LinkType string

const (
	LinkTypeInternal LinkType = "internal"
	LinkTypeExternal LinkType = "external"
	LinkTypeAnchor   LinkType = "anchor"
)

// LinkStatus represents the status of a link check
type LinkStatus string

const (
	LinkStatusOK     LinkStatus = "ok"
	LinkStatusBroken LinkStatus = "broken"
	LinkStatusSkipped LinkStatus = "skipped"
)

// Link represents a discovered link
type Link struct {
	URL      string   `json:"url"`
	Type     LinkType `json:"type"`
	SourceFile string `json:"source_file"`
	LineNumber int    `json:"line_number"`
}

// LinkResult represents the result of checking a link
type LinkResult struct {
	Link    Link       `json:"link"`
	Status  LinkStatus `json:"status"`
	Message string     `json:"message,omitempty"`
}

// Report contains the results of link validation
type Report struct {
	TotalLinks    int          `json:"total_links"`
	OKLinks       int          `json:"ok_links"`
	BrokenLinks   int          `json:"broken_links"`
	SkippedLinks  int          `json:"skipped_links"`
	Results       []LinkResult `json:"results"`
}
