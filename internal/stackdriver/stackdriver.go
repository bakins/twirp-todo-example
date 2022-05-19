package stackdriver

import (
	"runtime"
	"strconv"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Encoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "severity",
		NameKey:        "logger",
		CallerKey:      zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    encodeLevel,
		EncodeTime:     rfc3339NanoTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	return zapcore.NewJSONEncoder(encoderConfig)
}

var logLevelSeverity = map[zapcore.Level]string{
	zapcore.DebugLevel:  "DEBUG",
	zapcore.InfoLevel:   "INFO",
	zapcore.WarnLevel:   "WARNING",
	zapcore.ErrorLevel:  "ERROR",
	zapcore.DPanicLevel: "CRITICAL",
	zapcore.PanicLevel:  "ALERT",
	zapcore.FatalLevel:  "EMERGENCY",
}

func encodeLevel(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(logLevelSeverity[l])
}

func rfc3339NanoTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(time.RFC3339Nano))
}

const HTTPRequestField = "httpRequest"

func HTTP(req *HTTPRequest) zap.Field {
	return zap.Object(HTTPRequestField, req)
}

type HTTPRequest struct {
	RequestMethod string `json:"requestMethod"`
	RequestURL    string `json:"requestUrl"`
	RequestSize   string `json:"requestSize"`
	Latency       string `json:"latency"`
	ResponseSize  string `json:"responseSize"`
	UserAgent     string `json:"userAgent"`
	RemoteIP      string `json:"remoteIp"`
	ServerIP      string `json:"serverIp"`
	Referer       string `json:"referer"`
	Protocol      string `json:"protocol"`
	Status        int    `json:"status"`
}

func (req *HTTPRequest) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if req.RequestMethod != "" {
		enc.AddString("requestMethod", req.RequestMethod)
	}

	enc.AddString("requestUrl", req.RequestURL)
	enc.AddString("requestSize", req.RequestSize)

	if req.Status == 0 {
		enc.AddInt("status", req.Status)
	}
	enc.AddString("responseSize", req.ResponseSize)

	if req.UserAgent != "" {
		enc.AddString("userAgent", req.UserAgent)
	}

	if req.RemoteIP != "" {
		enc.AddString("remoteIp", req.RemoteIP)
	}

	if req.ServerIP != "" {
		enc.AddString("serverIp", req.ServerIP)
	}

	if req.Protocol != "" {
		enc.AddString("protocol", req.Protocol)
	}

	if req.Referer != "" {
		enc.AddString("referer", req.Referer)
	}

	if req.Latency != "" {
		enc.AddString("latency", req.Latency)
	}

	/*
		enc.AddBool("cacheLookup", req.CacheLookup)
		enc.AddBool("cacheHit", req.CacheHit)
		enc.AddBool("cacheValidatedWithOriginServer", req.CacheValidatedWithOriginServer)
		enc.AddString("cacheFillBytes", req.CacheFillBytes)
	*/
	return nil
}

const contextKey = "context"

// ErrorReport adds the correct Stackdriver "context" field for getting the log line
// reported as error.
//
// see: https://cloud.google.com/error-reporting/docs/formatting-error-messages
func ErrorReport(pc uintptr, file string, line int, ok bool) []zap.Field {
	fields := []zap.Field{
		// zap.String("@type", "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent"),
		zap.Object(contextKey, newReportContext(pc, file, line, ok)),
	}
	return fields
}

type reportLocation struct {
	File     string `json:"filePath"`
	Line     string `json:"lineNumber"`
	Function string `json:"functionName"`
}

func (location reportLocation) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("filePath", location.File)
	enc.AddString("lineNumber", location.Line)
	enc.AddString("functionName", location.Function)

	return nil
}

type reportContext struct {
	ReportLocation reportLocation `json:"reportLocation"`
}

func (context reportContext) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddObject("reportLocation", context.ReportLocation)

	return nil
}

func newReportContext(pc uintptr, file string, line int, ok bool) *reportContext {
	if !ok {
		return nil
	}

	var function string
	if fn := runtime.FuncForPC(pc); fn != nil {
		function = fn.Name()
	}

	context := &reportContext{
		ReportLocation: reportLocation{
			File:     file,
			Line:     strconv.Itoa(line),
			Function: function,
		},
	}

	return context
}

const serviceContextKey = "serviceContext"

// ServiceContext adds the correct service information adding the log line
// It is a required field if an error needs to be reported.
//
// see: https://cloud.google.com/error-reporting/reference/rest/v1beta1/ServiceContext
// see: https://cloud.google.com/error-reporting/docs/formatting-error-messages
func ServiceContext(name, version string) zap.Field {
	return zap.Object(serviceContextKey, newServiceContext(name, version))
}

// serviceContext describes a running service that sends errors.
// Currently it only describes a service name.
type serviceContext struct {
	Name    string `json:"service"`
	Version string `json:"version"`
}

// MarshalLogObject implements zapcore.ObjectMarshaller interface.
func (service_context *serviceContext) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("service", service_context.Name)
	enc.AddString("version", service_context.Version)
	return nil
}

func newServiceContext(name, version string) *serviceContext {
	return &serviceContext{
		Name:    name,
		Version: version,
	}
}

type Core struct {
	core    zapcore.Core
	service *serviceContext
}

func WrapCore(core zapcore.Core, serviceName string, serviceVersion string) *Core {
	if serviceName == "" {
		serviceName = "unknown"
	}

	if serviceVersion == "" {
		serviceVersion = "unknown"
	}

	c := Core{
		core:    core,
		service: newServiceContext(serviceName, serviceVersion),
	}

	return &c
}

func (c *Core) With(fields []zap.Field) zapcore.Core {
	core := c.core.With(fields)

	newcore := Core{
		core:    core,
		service: c.service,
	}

	return &newcore
}

func (c *Core) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}

	return ce
}

func (c *Core) Enabled(l zapcore.Level) bool {
	return c.core.Enabled(l)
}

func (c *Core) Sync() error {
	return c.core.Sync()
}

func (c *Core) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	fields = c.withServiceContext(fields)

	if zapcore.ErrorLevel.Enabled(ent.Level) {
		fields = c.withSourceLocation(ent, fields)
		fields = c.withErrorReport(ent, fields)
	}

	return c.core.Write(ent, fields)
}

func (c *Core) withServiceContext(fields []zapcore.Field) []zapcore.Field {
	// If the service context was manually set, don't overwrite it
	for i := range fields {
		if fields[i].Key == serviceContextKey {
			return fields
		}
	}

	return append(fields, zap.Object(serviceContextKey, c.service))
}

func (c *Core) withErrorReport(ent zapcore.Entry, fields []zapcore.Field) []zapcore.Field {
	// If the error report was manually set, don't overwrite it
	for i := range fields {
		if fields[i].Key == contextKey {
			return fields
		}
	}

	if !ent.Caller.Defined {
		return fields
	}

	return append(fields, ErrorReport(ent.Caller.PC, ent.Caller.File, ent.Caller.Line, true)...)
}

func (c *Core) withSourceLocation(ent zapcore.Entry, fields []zapcore.Field) []zapcore.Field {
	// If the source location was manually set, don't overwrite it
	for i := range fields {
		if fields[i].Key == sourceKey {
			return fields
		}
	}

	if !ent.Caller.Defined {
		return fields
	}

	return append(fields, SourceLocation(ent.Caller.PC, ent.Caller.File, ent.Caller.Line, true))
}

const sourceKey = "logging.googleapis.com/sourceLocation"

// SourceLocation adds the correct Stackdriver "SourceLocation" field.
//
// see: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogEntrySourceLocation
func SourceLocation(pc uintptr, file string, line int, ok bool) zap.Field {
	return zap.Object(sourceKey, newSource(pc, file, line, ok))
}

// source is the source code location information associated with the log entry,
// if any.
type source struct {
	// Optional. Source file name. Depending on the runtime environment, this
	// might be a simple name or a fully-qualified name.
	File string `json:"file"`

	// Optional. Line within the source file. 1-based; 0 indicates no line number
	// available.
	Line string `json:"line"`

	// Optional. Human-readable name of the function or method being invoked, with
	// optional context such as the class or package name. This information may be
	// used in contexts such as the logs viewer, where a file and line number are
	// less meaningful.
	//
	// The format should be dir/package.func.
	Function string `json:"function"`
}

// MarshalLogObject implements zapcore.ObjectMarshaller interface.
func (source source) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("file", source.File)
	enc.AddString("line", source.Line)
	enc.AddString("function", source.Function)

	return nil
}

func newSource(pc uintptr, file string, line int, ok bool) *source {
	if !ok {
		return nil
	}

	var function string
	if fn := runtime.FuncForPC(pc); fn != nil {
		function = fn.Name()
	}

	source := &source{
		File:     file,
		Line:     strconv.Itoa(line),
		Function: function,
	}

	return source
}
