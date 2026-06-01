package sse

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WriteEvent writes a properly formatted Server-Sent Event to the writer.
// Handles multiline data by prefixing each line with "data: "
func WriteEvent(w io.Writer, eventType, data string) {
	if eventType != "" {
		fmt.Fprintf(w, "event: %s\n", eventType)
	}

	// Handle multiline data - each line needs "data: " prefix
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	for _, line := range lines {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprintf(w, "\n")

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
