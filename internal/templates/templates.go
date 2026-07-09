package templates

import (
	"embed"
	"html/template"
)

//go:embed report.html ui.html
var files embed.FS

// Report is the email/web report template (executed with a service.ReportView).
var Report = template.Must(template.ParseFS(files, "report.html"))

// UIHTML returns the raw config dashboard page. Served as-is (it contains
// vanilla JS), so it is not parsed as a Go template.
func UIHTML() ([]byte, error) {
	return files.ReadFile("ui.html")
}
