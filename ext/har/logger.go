package har

import (
    "net/http"
    "sync"
    "time"
    "github.com/elazarl/goproxy"
)

// ExportFunc is a function type that users can implement to handle exported entries
type ExportFunc func([]Entry)

// Logger implements a HAR logging extension for goproxy
type Logger struct {
    mu             sync.Mutex
    entries        []Entry
    captureContent bool
    exportFunc     ExportFunc
    exportInterval time.Duration
    exportThreashold    int
    stopChan       chan struct{}
}

// LoggerOption is a function type for configuring the Logger
type LoggerOption func(*Logger)

// WithExportCount sets the number of requests after which to export entries
func WithExportThreshold(threshold int) LoggerOption {
    return func(l *Logger) {
        l.exportThreashold = threshold
    }
}

// NewLogger creates a new HAR logger instance
func NewLogger(exportFunc ExportFunc, opts ...LoggerOption) *Logger {
    l := &Logger{
        entries:        make([]Entry, 0),
        captureContent: true,
        exportFunc:     exportFunc,
        stopChan:       make(chan struct{}),
    }

    for _, opt := range opts {
        opt(l)
    }

    go l.exportLoop()

    return l
}

// OnRequest handles incoming HTTP requests
func (l *Logger) OnRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
    if ctx != nil {
        ctx.UserData = time.Now()
    }
    return req, nil
}

// OnResponse handles HTTP responses
func (l *Logger) OnResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
    if resp == nil || ctx == nil || ctx.Req == nil || ctx.UserData == nil {
        return resp
    }
    startTime, ok := ctx.UserData.(time.Time)
    if !ok {
        return resp
    }
    
    entry := Entry{
        StartedDateTime: startTime,
        Time:            time.Since(startTime).Milliseconds(),
        Request:         ParseRequest(ctx.Req, l.captureContent),
        Response:        ParseResponse(resp, l.captureContent),
        Timings: Timings{
            Send:    0,
            Wait:    time.Since(startTime).Milliseconds(),
            Receive: 0,
        },
    }
    entry.fillIPAddress(ctx.Req)
    
    l.mu.Lock()
    l.entries = append(l.entries, entry)
    l.mu.Unlock()
    
    return resp
}

func (l *Logger) exportLoop() {
    ticker := time.NewTicker(100 * time.Millisecond) // Check frequently
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            l.checkAndExport()
        case <-l.stopChan:
            return
        }
    }
}

func (l *Logger) checkAndExport() {
    l.mu.Lock()
    defer l.mu.Unlock()

    if l.exportThreashold > 0 && len(l.entries) >= l.exportThreashold {
        l.exportFunc(l.entries)
        l.entries = make([]Entry, 0)
    }
}

// Stop stops the export loop
func (l *Logger) Stop() {
    close(l.stopChan)
}
