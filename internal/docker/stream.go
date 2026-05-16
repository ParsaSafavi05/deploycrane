package docker

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
)

// BuildLogEntry is the JSON structure Docker returns.
type BuildLogEntry struct {
    Stream string `json:"stream"`
    Error  string `json:"error"`
    Aux    struct {
        ID string `json:"ID"`
    } `json:"aux"`
}

// StreamBuildLogs reads the raw Docker build response and pushes
// each log line as a plain-text event to the provided writer,
// which can be an http.ResponseWriter using SSE.
func StreamBuildLogs(dst io.Writer, src io.Reader) error {
    scanner := bufio.NewScanner(src)
    for scanner.Scan() {
        var entry BuildLogEntry
        if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
            // Non-JSON line – could be a raw message, forward it anyway
            fmt.Fprintf(dst, "data: %s\n\n", scanner.Text())
            continue
        }
        if entry.Error != "" {
            fmt.Fprintf(dst, "event: error\ndata: %s\n\n", entry.Error)
            return fmt.Errorf("build error: %s", entry.Error)
        }
        if entry.Stream != "" {
            fmt.Fprintf(dst, "data: %s\n\n", entry.Stream)
        }
        // 'aux' with ID is just informational – ignore or send as comment
    }
    if err := scanner.Err(); err != nil {
        return fmt.Errorf("reading build stream: %w", err)
    }
    // Send a final "complete" event
    fmt.Fprintf(dst, "event: complete\ndata: build finished\n\n")
    return nil
}