package trifle

import (
	"bytes"
	"context"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	testing "github.com/mitchellh/go-testing-interface"
	"miren.dev/trifle/pkg/color"
)

var (
	Trace = slog.LevelDebug - 4
	Debug = slog.LevelDebug
	Info  = slog.LevelInfo
	Warn  = slog.LevelWarn
	Error = slog.LevelError

	_levelToName = map[slog.Level]string{
		Trace:           " [TRACE] ",
		slog.LevelDebug: " [DEBUG] ",
		slog.LevelInfo:  " [INFO]  ",
		slog.LevelWarn:  " [WARN]  ",
		slog.LevelError: " [ERROR] ",
	}

	_levelToColor = map[slog.Level]*color.Color{
		Trace:           color.New(color.FgHiGreen),
		slog.LevelDebug: color.New(color.FgHiWhite),
		slog.LevelInfo:  color.New(color.FgHiBlue),
		slog.LevelWarn:  color.New(color.FgHiYellow),
		slog.LevelError: color.New(color.FgHiRed),
	}

	moduleColor       = color.New(color.Faint)
	importantKeyColor = color.New(color.FgHiYellow)
	criticalKeyColor  = color.New(color.FgHiRed)
	contextColor      = color.New(color.Faint)
)

// TextHandler is a [Handler] that writes Records to an [io.Writer] as a
// sequence of key=value pairs separated by spaces and followed by a newline.
type TextHandler struct {
	*commonHandler

	module string
}

// Option is a function that configures a TextHandler.
type Option func(*TextHandler)

// WithImportantKeys returns an Option that marks the specified keys as important.
// Important keys will be highlighted in yellow when printed.
func WithImportantKeys(keys ...string) Option {
	return func(h *TextHandler) {
		if h.importantKeys == nil {
			h.importantKeys = make(map[string]bool)
		}
		for _, key := range keys {
			h.importantKeys[key] = true
		}
	}
}

// WithCriticalKeys returns an Option that marks the specified keys as critical.
// Critical keys will be highlighted in red when printed.
func WithCriticalKeys(keys ...string) Option {
	return func(h *TextHandler) {
		if h.criticalKeys == nil {
			h.criticalKeys = make(map[string]bool)
		}
		for _, key := range keys {
			h.criticalKeys[key] = true
		}
	}
}

// WithContextKey returns an Option that sets keys whose values will be displayed
// before the message without the key names, in a subdued color. Multiple keys
// are displayed in the order specified, separated by spaces. Missing keys are skipped.
func WithContextKey(keys ...string) Option {
	return func(h *TextHandler) {
		h.contextKeys = keys
	}
}

// WithTerminalWidth sets a specific terminal width for word wrapping
// (mainly useful for testing)
func WithTerminalWidth(width int) Option {
	return func(h *TextHandler) {
		h.terminalWidth = width
	}
}

// New creates a [TextHandler] that writes to w,
// using the given options.
// If opts is nil, the default options are used.
func New(w io.Writer, opts *slog.HandlerOptions, options ...Option) *TextHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}

	termWidth := getTerminalWidth(w)
	h := &TextHandler{
		commonHandler: &commonHandler{
			w:             w,
			opts:          *opts,
			mu:            &sync.Mutex{},
			terminalWidth: termWidth,
		},
		module: "",
	}

	// Apply options
	for _, opt := range options {
		opt(h)
	}

	return h
}

// Quick returns a [TextHandler] that writes to os.Stderr at the Debug level.
func Quick() *TextHandler {
	return New(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
}

// Enabled reports whether the handler handles records at the given level.
// The handler ignores records whose level is lower.
func (h *TextHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.enabled(level)
}

const ModuleKey = "module"

// WithAttrs returns a new [TextHandler] whose attributes consists
// of h's attributes followed by attrs.
func (h *TextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var (
		goodAttrs []slog.Attr
		module    = h.module
	)

	for _, a := range attrs {
		if a.Key == ModuleKey && a.Value.Kind() == slog.KindString {
			if module == "" {
				module = a.Value.String()
			} else {
				module += "." + a.Value.String()
			}
		} else {
			goodAttrs = append(goodAttrs, a)
		}
	}

	return &TextHandler{commonHandler: h.withAttrs(goodAttrs), module: module}
}

func (h *TextHandler) WithGroup(name string) slog.Handler {
	return &TextHandler{commonHandler: h.withGroup(name), module: h.module}
}

// Handle formats its argument [Record] as a single line of space-separated
// key=value items.
//
// If the Record's time is zero, the time is omitted.
// Otherwise, the key is "time"
// and the value is output in RFC3339 format with millisecond precision.
//
// If the Record's level is zero, the level is omitted.
// Otherwise, the key is "level"
// and the value of [Level.String] is output.
//
// If the AddSource option is set and source information is available,
// the key is "source" and the value is output as FILE:LINE.
//
// The message's key is "msg".
//
// To modify these or other attributes, or remove them from the output, use
// [HandlerOptions.ReplaceAttr].
//
// If a value implements [encoding.TextMarshaler], the result of MarshalText is
// written. Otherwise, the result of [fmt.Sprint] is written.
//
// Keys and values are quoted with [strconv.Quote] if they contain Unicode space
// characters, non-printing characters, '"' or '='.
//
// Keys inside groups consist of components (keys or group names) separated by
// dots. No further escaping is performed.
// Thus there is no way to determine from the key "a.b.c" whether there
// are two groups "a" and "b" and a key "c", or a single group "a.b" and a key "c",
// or single group "a" and a key "b.c".
// If it is necessary to reconstruct the group structure of a key
// even in the presence of dots inside components, use
// [HandlerOptions.ReplaceAttr] to encode that information in the key.
//
// Each call to Handle results in a single serialized call to
// io.Writer.Write.
func (h *TextHandler) Handle(_ context.Context, r slog.Record) error {
	return h.handle(r, h.module)
}

type commonHandler struct {
	opts              slog.HandlerOptions
	preformattedAttrs []byte
	// groupPrefix is for the text handler only.
	// It holds the prefix for groups that were already pre-formatted.
	// A group will appear here when a call to WithGroup is followed by
	// a call to WithAttrs.
	groupPrefix   string
	groups        []string // all groups started from WithGroup
	nOpenGroups   int      // the number of groups opened in preformattedAttrs
	mu            *sync.Mutex
	w             io.Writer
	importantKeys map[string]bool
	criticalKeys  map[string]bool
	contextKeys   []string
	contextValues map[string]string // cached context values from preformatted attrs
	terminalWidth int               // terminal width for word wrapping

	lastTime atomic.Int64
}

func (h *commonHandler) clone() *commonHandler {
	// We can't use assignment because we can't copy the mutex.
	cloned := &commonHandler{
		opts:              h.opts,
		preformattedAttrs: slices.Clip(h.preformattedAttrs),
		groupPrefix:       h.groupPrefix,
		groups:            slices.Clip(h.groups),
		nOpenGroups:       h.nOpenGroups,
		w:                 h.w,
		mu:                h.mu, // mutex shared among all clones of this handler
		importantKeys:     h.importantKeys,
		criticalKeys:      h.criticalKeys,
		contextKeys:       slices.Clip(h.contextKeys),
		terminalWidth:     h.terminalWidth,
	}
	// Deep copy the context values map
	if h.contextValues != nil {
		cloned.contextValues = make(map[string]string)
		for k, v := range h.contextValues {
			cloned.contextValues[k] = v
		}
	}
	return cloned
}

// enabled reports whether l is greater than or equal to the
// minimum level.
func (h *commonHandler) enabled(l slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return l >= minLevel
}

func (h *commonHandler) withAttrs(as []slog.Attr) *commonHandler {
	// We are going to ignore empty groups, so if the entire slice consists of
	// them, there is nothing to do.
	if countEmptyGroups(as) == len(as) {
		return h
	}
	h2 := h.clone()

	// Check if any context keys are being added
	if len(h2.contextKeys) > 0 {
		if h2.contextValues == nil {
			h2.contextValues = make(map[string]string)
		}
		for _, a := range as {
			for _, contextKey := range h2.contextKeys {
				if a.Key == contextKey && h2.contextValues[contextKey] == "" {
					h2.contextValues[contextKey] = fmt.Sprint(a.Value.Any())
				}
			}
		}
	}

	// Filter out context keys from attributes if they exist
	if len(h2.contextKeys) > 0 {
		var filtered []slog.Attr
		for _, a := range as {
			isContextKey := false
			for _, contextKey := range h2.contextKeys {
				if a.Key == contextKey {
					isContextKey = true
					break
				}
			}
			if !isContextKey {
				filtered = append(filtered, a)
			}
		}
		as = filtered
	}

	// Pre-format the attributes as an optimization.
	state := h2.newHandleState((*Buffer)(&h2.preformattedAttrs), false, "")
	defer state.free()
	state.prefix.WriteString(h.groupPrefix)
	if pfa := h2.preformattedAttrs; len(pfa) > 0 {
		state.sep = h.attrSep()
	}
	// Remember the position in the buffer, in case all attrs are empty.
	pos := state.buf.Len()
	state.openGroups()
	if !state.appendAttrs(as) {
		state.buf.SetLen(pos)
	} else {
		// Remember the new prefix for later keys.
		h2.groupPrefix = state.prefix.String()
		// Remember how many opened groups are in preformattedAttrs,
		// so we don't open them again when we handle a Record.
		h2.nOpenGroups = len(h2.groups)
	}
	return h2
}

func (h *commonHandler) withGroup(name string) *commonHandler {
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

// source returns a Source for the log event.
// If the Record was created without the necessary information,
// or if the location is unavailable, it returns a non-nil *Source
// with zero fields.
func recordSource(r slog.Record) *slog.Source {
	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()
	return &slog.Source{
		Function: f.Function,
		File:     f.File,
		Line:     f.Line,
	}
}

// handle is the internal implementation of Handler.Handle
// used by TextHandler and JSONHandler.
func (h *commonHandler) handle(r slog.Record, module string) error {
	state := h.newHandleState(NewBuffer(), true, "")
	defer state.free()
	// Built-in attributes. They are not in a group.
	stateGroups := state.groups
	state.groups = nil // So ReplaceAttrs sees no groups instead of the pre groups.
	rep := h.opts.ReplaceAttr

	// time
	if !r.Time.IsZero() {
		key := slog.TimeKey
		val := r.Time.Round(0) // strip monotonic to match Attr behavior

		if rep == nil {
			lastTime := time.Unix(h.lastTime.Load(), 0)

			if lastTime.IsZero() || val.Sub(lastTime) < 1*time.Hour || val.YearDay() != lastTime.YearDay() {
				state.appendMiniTime(val)
			} else {
				state.appendShortTime(val)
			}
			h.lastTime.Store(val.Unix())
		} else {
			state.appendAttr(slog.Time(key, val))
		}
	}

	// level
	key := slog.LevelKey
	val := r.Level
	str := val.String()

	spec, ok := _levelToName[r.Level]
	if ok {
		str = spec
	}

	if col, ok := _levelToColor[val]; ok {
		str = col.Sprint(str)
	}

	state.appendRawString(str)

	// source
	if h.opts.AddSource {
		state.appendAttr(slog.Any(slog.SourceKey, recordSource(r)))
		state.appendRawString(" ")
	}

	// Extract and display context values if contextKeys are set
	if len(h.contextKeys) > 0 {
		var contextParts []string

		// Build a map of available values from record attrs
		recordValues := make(map[string]string)
		r.Attrs(func(a slog.Attr) bool {
			for _, contextKey := range h.contextKeys {
				if a.Key == contextKey {
					recordValues[contextKey] = fmt.Sprint(a.Value.Any())
				}
			}
			return true
		})

		// Collect values in the order specified by contextKeys
		for _, contextKey := range h.contextKeys {
			// Check cached values first
			if h.contextValues != nil {
				if val, ok := h.contextValues[contextKey]; ok && val != "" {
					contextParts = append(contextParts, val)
					continue
				}
			}
			// Then check record values
			if val, ok := recordValues[contextKey]; ok {
				contextParts = append(contextParts, val)
			}
		}

		// Display all found context values
		if len(contextParts) > 0 {
			state.appendRawString(contextColor.Sprint(strings.Join(contextParts, " ")))
			state.appendRawString(" ")
		}
	}

	if module != "" {
		state.appendRawString(moduleColor.Sprint(module))
		state.appendRawString(" ")
	}

	key = slog.MessageKey
	msg := r.Message
	if rep == nil {
		state.appendRawString(msg)
		if r.NumAttrs() > 0 || len(state.h.preformattedAttrs) > 0 {
			state.appendRawString(" │ ")
		}
	} else {
		state.appendAttr(slog.String(key, msg))
	}
	state.groups = stateGroups // Restore groups passed to ReplaceAttrs.
	state.appendNonBuiltIns(r)
	state.buf.WriteNewLine()

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(*state.buf)
	return err
}

func (s *handleState) appendNonBuiltIns(r slog.Record) {
	// preformatted Attrs
	if pfa := s.h.preformattedAttrs; len(pfa) > 0 {
		s.buf.WriteString(s.sep)
		s.buf.Write(pfa)
		s.sep = s.h.attrSep()
	}
	// Attrs in Record -- unlike the built-in ones, they are in groups started
	// from WithGroup.
	// If the record has no Attrs, don't output any groups.
	if r.NumAttrs() > 0 {
		s.prefix.WriteString(s.h.groupPrefix)
		// The group may turn out to be empty even though it has attrs (for
		// example, ReplaceAttr may delete all the attrs).
		// So remember where we are in the buffer, to restore the position
		// later if necessary.
		pos := s.buf.Len()
		s.openGroups()
		empty := true
		r.Attrs(func(a slog.Attr) bool {
			if s.appendAttr(a) {
				empty = false
			}
			return true
		})
		if empty {
			s.buf.SetLen(pos)
		}
	}
}

// attrSep returns the separator between attributes.
func (h *commonHandler) attrSep() string {
	return " "
}

// handleState holds state for a single call to commonHandler.handle.
// The initial value of sep determines whether to emit a separator
// before the next key, after which it stays true.
type handleState struct {
	h           *commonHandler
	buf         *Buffer
	freeBuf     bool      // should buf be freed?
	sep         string    // separator to write before next key
	prefix      *Buffer   // for text: key prefix
	groups      *[]string // pool-allocated slice of active groups, for ReplaceAttr
	linePos     int       // current position on the line for word wrapping
	needsIndent bool      // whether next output needs indentation
}

var groupPool = sync.Pool{New: func() any {
	s := make([]string, 0, 10)
	return &s
}}

func (h *commonHandler) newHandleState(buf *Buffer, freeBuf bool, sep string) handleState {
	s := handleState{
		h:           h,
		buf:         buf,
		freeBuf:     freeBuf,
		sep:         sep,
		prefix:      NewBuffer(),
		linePos:     0,
		needsIndent: false,
	}
	if h.opts.ReplaceAttr != nil {
		s.groups = groupPool.Get().(*[]string)
		*s.groups = append(*s.groups, h.groups[:h.nOpenGroups]...)
	}
	return s
}

func (s *handleState) free() {
	if s.freeBuf {
		s.buf.Free()
	}
	if gs := s.groups; gs != nil {
		*gs = (*gs)[:0]
		groupPool.Put(gs)
	}
	s.prefix.Free()
}

func (s *handleState) openGroups() {
	for _, n := range s.h.groups[s.h.nOpenGroups:] {
		s.openGroup(n)
	}
}

// Separator for group names and keys.
const keyComponentSep = '.'

// openGroup starts a new group of attributes
// with the given name.
func (s *handleState) openGroup(name string) {
	s.prefix.WriteString(name)
	s.prefix.WriteByte(keyComponentSep)
	// Collect group names for ReplaceAttr.
	if s.groups != nil {
		*s.groups = append(*s.groups, name)
	}
}

// closeGroup ends the group with the given name.
func (s *handleState) closeGroup(name string) {
	(*s.prefix) = (*s.prefix)[:len(*s.prefix)-len(name)-1 /* for keyComponentSep */]
	s.sep = s.h.attrSep()
	if s.groups != nil {
		*s.groups = (*s.groups)[:len(*s.groups)-1]
	}
}

// appendAttrs appends the slice of Attrs.
// It reports whether something was appended.
func (s *handleState) appendAttrs(as []slog.Attr) bool {
	nonEmpty := false
	for _, a := range as {
		if s.appendAttr(a) {
			nonEmpty = true
		}
	}
	return nonEmpty
}

func isEmptyGroup(v slog.Value) bool {
	if v.Kind() != slog.KindGroup {
		return false
	}
	// We do not need to recursively examine the group's Attrs for emptiness,
	// because GroupValue removed them when the group was constructed, and
	// groups are immutable.
	return len(v.Group()) == 0
}

// countEmptyGroups returns the number of empty group values in its argument.
func countEmptyGroups(as []slog.Attr) int {
	n := 0
	for _, a := range as {
		if isEmptyGroup(a.Value) {
			n++
		}
	}
	return n
}

// isEmpty reports whether a has an empty key and a nil value.
// That can be written as Attr{} or Any("", nil).
func isEmpty(a slog.Attr) bool {
	a.Value.Kind()
	return a.Key == "" && a.Value.Equal(slog.Value{})
}

// group returns the non-zero fields of s as a slice of attrs.
// It is similar to a LogValue method, but we don't want Source
// to implement LogValuer because it would be resolved before
// the ReplaceAttr function was called.
func sourceGroup(s *slog.Source) slog.Value {
	var as []slog.Attr
	if s.Function != "" {
		as = append(as, slog.String("function", s.Function))
	}
	if s.File != "" {
		as = append(as, slog.String("file", s.File))
	}
	if s.Line != 0 {
		as = append(as, slog.Int("line", s.Line))
	}
	return slog.GroupValue(as...)
}

// appendAttr appends the Attr's key and value.
// It handles replacement and checking for an empty key.
// It reports whether something was appended.
func (s *handleState) appendAttr(a slog.Attr) bool {
	// Skip context keys if they're being displayed separately
	if len(s.h.contextKeys) > 0 && (s.groups == nil || len(*s.groups) == 0) {
		for _, contextKey := range s.h.contextKeys {
			if a.Key == contextKey {
				return false
			}
		}
	}

	a.Value = a.Value.Resolve()
	if rep := s.h.opts.ReplaceAttr; rep != nil && a.Value.Kind() != slog.KindGroup {
		var gs []string
		if s.groups != nil {
			gs = *s.groups
		}
		// a.Value is resolved before calling ReplaceAttr, so the user doesn't have to.
		a = rep(gs, a)
		// The ReplaceAttr function may return an unresolved Attr.
		a.Value = a.Value.Resolve()
	}
	// Elide empty Attrs.
	if isEmpty(a) {
		return false
	}
	// Special case: Source.
	if v := a.Value; v.Kind() == slog.KindAny {
		if src, ok := v.Any().(*slog.Source); ok {
			a.Value = slog.StringValue(fmt.Sprintf("%s:%d", src.File, src.Line))
		}
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		// Output only non-empty groups.
		if len(attrs) > 0 {
			// The group may turn out to be empty even though it has attrs (for
			// example, ReplaceAttr may delete all the attrs).
			// So remember where we are in the buffer, to restore the position
			// later if necessary.
			pos := s.buf.Len()
			// Inline a group with an empty key.
			if a.Key != "" {
				s.openGroup(a.Key)
			}
			if !s.appendAttrs(attrs) {
				s.buf.SetLen(pos)
				return false
			}
			if a.Key != "" {
				s.closeGroup(a.Key)
			}
		}
	} else {
		if a.Value.Kind() == slog.KindString {
			str := a.Value.String()
			if strings.Contains(str, "\n") {
				s.appendKey(a.Key)
				s.appendRawString("\n")
				writeIndent(s, str, "  │ ")
				return true
			}
		}

		// For wrapping: check if key + value would fit on current line
		if s.h.terminalWidth > 0 && s.linePos > 4 { // Only if not at start of indented line
			// Estimate the total length of key + value
			sepLen := 0
			if s.sep != "" {
				sepLen = calculateVisibleLength(s.sep)
			}
			keyLen := len(a.Key) + 2 // key + ": "

			// Estimate value length
			valueLen := estimateValueLength(a.Value)

			// Check if the entire key-value pair would overflow
			totalLen := sepLen + keyLen + valueLen
			if s.linePos+totalLen > s.h.terminalWidth {
				// Wrap to new line before key
				s.buf.WriteNewLine()
				s.buf.WriteString("    ")
				s.linePos = 4
				s.sep = "" // No separator needed at start of new line
			}
		}

		s.appendKey(a.Key)
		s.appendValue(a.Value)
	}
	return true
}

// TimeFormat is the time format to use for plain (non-JSON) output.
// This is a version of RFC3339 that contains millisecond precision.
const (
	TimeFormat     = "2006-01-02T15:04:05.000Z0700"
	MiniTimeFormat = "15:04:05.000"
)

func (s *handleState) appendShortTime(t time.Time) {
	str := t.Format(TimeFormat)
	s.buf.WriteString(str)
	s.linePos += len(str)
}

func (s *handleState) appendMiniTime(t time.Time) {
	str := t.Format(MiniTimeFormat)
	s.buf.WriteString(str)
	s.linePos += len(str)
}

func (s *handleState) appendError(err error) {
	s.appendString(fmt.Sprintf("!ERROR:%v", err))
}

var (
	faintBoldColor = color.New(color.Faint, color.Bold)
	boldColor      = color.New(color.Bold)
)

// calculateVisibleLength estimates the visible length of a string, ignoring ANSI codes
func calculateVisibleLength(s string) int {
	// Simple approximation: strip ANSI codes
	inCode := false
	length := 0
	for _, r := range s {
		if r == '\x1b' {
			inCode = true
		} else if inCode && r == 'm' {
			inCode = false
		} else if !inCode {
			length++
		}
	}
	return length
}

// estimateValueLength estimates the length a value will take when printed
func estimateValueLength(v slog.Value) int {
	switch v.Kind() {
	case slog.KindString:
		str := v.String()
		if needsQuoting(str) {
			return len(strconv.Quote(str))
		}
		return len(str)
	case slog.KindInt64:
		return len(strconv.FormatInt(v.Int64(), 10))
	case slog.KindFloat64:
		return len(strconv.FormatFloat(v.Float64(), 'g', -1, 64))
	case slog.KindBool:
		if v.Bool() {
			return 4 // "true"
		}
		return 5 // "false"
	case slog.KindDuration:
		return len(v.Duration().String())
	case slog.KindTime:
		// RFC3339 format is fairly consistent in length
		return 20
	default:
		// For other types, make a reasonable estimate
		return 20
	}
}

func (s *handleState) appendKey(key string) {
	// Write separator if needed
	if s.sep != "" {
		s.buf.WriteString(s.sep)
		s.linePos += calculateVisibleLength(s.sep)
	}

	// Track visible key length before adding colors
	visibleKeyLen := len(key) + 2 // key + ": "

	// Check key priority: critical > important > normal
	if s.h.criticalKeys != nil && s.h.criticalKeys[key] {
		key = criticalKeyColor.Colorize(key) + boldColor.Colorize(": ")
	} else if s.h.importantKeys != nil && s.h.importantKeys[key] {
		key = importantKeyColor.Colorize(key) + boldColor.Colorize(": ")
	} else {
		key = faintBoldColor.Colorize(key) + boldColor.Colorize(": ")
	}

	if s.prefix != nil && len(*s.prefix) > 0 {
		prefixLen := len(*s.prefix)
		s.buf.Write(*s.prefix)
		s.buf.WriteString(key)
		s.linePos += prefixLen + visibleKeyLen
	} else {
		s.buf.WriteString(key)
		s.linePos += visibleKeyLen
	}
	s.sep = s.h.attrSep()
}

func needsQuoting(s string) bool {
	if len(s) == 0 {
		return true
	}
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			// Quote anything except a backslash that would need quoting in a
			// JSON string, as well as space and '='
			if b != '\\' && (b == ' ' || b == '=' || !safeSet[b]) {
				return true
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError || unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return true
		}
		i += size
	}
	return false
}

func (s *handleState) appendRawString(str string) {
	// Handle any needed indentation
	if s.needsIndent && s.h.terminalWidth > 0 {
		s.buf.WriteString("    ")
		s.linePos = 4
		s.needsIndent = false
	}

	s.buf.WriteString(str)
	// Track line position using visible length
	s.linePos += calculateVisibleLength(str)
}

func (s *handleState) appendString(str string) {
	// Check if value would cause overflow before writing
	valueLen := len(str)
	if needsQuoting(str) {
		valueLen = len(strconv.Quote(str))
	}

	// If terminal width is set and the value would overflow, wrap first
	if s.h.terminalWidth > 0 && s.linePos > 0 && s.linePos+valueLen > s.h.terminalWidth {
		s.buf.WriteNewLine()
		s.buf.WriteString("    ")
		s.linePos = 4
	}

	// text
	if strings.Contains(str, "\n") {
		s.appendRawString("\n")
		writeIndent(s, str, "  │ ")
		s.linePos = 0 // Reset after newline
		return
	}

	if needsQuoting(str) {
		quoted := strconv.Quote(str)
		s.buf.WriteString(quoted)
		s.linePos += len(quoted)
	} else {
		s.buf.WriteString(str)
		s.linePos += len(str)
	}
}

// byteSlice returns its argument as a []byte if the argument's
// underlying type is []byte, along with a second return value of true.
// Otherwise it returns nil, false.
func byteSlice(a any) ([]byte, bool) {
	if bs, ok := a.([]byte); ok {
		return bs, true
	}
	// Like Printf's %s, we allow both the slice type and the byte element type to be named.
	t := reflect.TypeOf(a)
	if t != nil && t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		return reflect.ValueOf(a).Bytes(), true
	}
	return nil, false
}

// append appends a text representation of v to dst.
// v is formatted as with fmt.Sprint.
func appendValue(v slog.Value, dst []byte) []byte {
	switch v.Kind() {
	case slog.KindString:
		return append(dst, v.String()...)
	case slog.KindInt64:
		return strconv.AppendInt(dst, v.Int64(), 10)
	case slog.KindUint64:
		return strconv.AppendUint(dst, v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.AppendFloat(dst, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		return strconv.AppendBool(dst, v.Bool())
	case slog.KindDuration:
		return append(dst, v.Duration().String()...)
	case slog.KindTime:
		return append(dst, v.Time().String()...)
	case slog.KindGroup:
		return fmt.Append(dst, v.Group())
	case slog.KindAny, slog.KindLogValuer:
		return fmt.Append(dst, v.Any())
	default:
		panic(fmt.Sprintf("bad kind: %s", v.Kind()))
	}
}

func writeIndent(w *handleState, str string, indent string) {
	for {
		nl := strings.IndexByte(str, '\n')
		if nl == -1 {
			if str != "" {
				w.appendRawString(indent)
				writeEscapedForOutput(w, str, false)
				w.appendRawString("\n")
			}
			return
		}

		w.appendRawString(indent)
		writeEscapedForOutput(w, str[:nl], false)
		w.appendRawString("\n")
		str = str[nl+1:]
	}
}
func appendTextValue(s *handleState, v slog.Value) error {
	switch v.Kind() {
	case slog.KindString:
		str := v.String()
		if strings.Contains(str, "\n") {
			s.appendRawString("\n  ")
		} else {
			s.appendString(v.String())
		}
	case slog.KindTime:
		s.appendTime(v.Time())
	case slog.KindAny:
		if tm, ok := v.Any().(encoding.TextMarshaler); ok {
			data, err := tm.MarshalText()
			if err != nil {
				return err
			}
			// TODO: avoid the conversion to string.
			s.appendString(string(data))
			return nil
		}
		if bs, ok := byteSlice(v.Any()); ok {
			// As of Go 1.19, this only allocates for strings longer than 32 bytes.
			s.buf.WriteString(strconv.Quote(string(bs)))
			return nil
		}
		s.appendString(fmt.Sprintf("%+v", v.Any()))
	default:
		*s.buf = appendValue(v, *s.buf)
	}
	return nil
}

func (s *handleState) appendValue(v slog.Value) {
	defer func() {
		if r := recover(); r != nil {
			// If it panics with a nil pointer, the most likely cases are
			// an encoding.TextMarshaler or error fails to guard against nil,
			// in which case "<nil>" seems to be the feasible choice.
			//
			// Adapted from the code in fmt/print.go.
			if v := reflect.ValueOf(v.Any()); v.Kind() == reflect.Pointer && v.IsNil() {
				s.appendString("<nil>")
				return
			}

			// Otherwise just print the original panic message.
			s.appendString(fmt.Sprintf("!PANIC: %v", r))
		}
	}()

	err := appendTextValue(s, v)
	if err != nil {
		s.appendError(err)
	}
}

func (s *handleState) appendTime(t time.Time) {
	*s.buf = appendRFC3339Millis(*s.buf, t)
}

func appendRFC3339Millis(b []byte, t time.Time) []byte {
	// Format according to time.RFC3339Nano since it is highly optimized,
	// but truncate it to use millisecond resolution.
	// Unfortunately, that format trims trailing 0s, so add 1/10 millisecond
	// to guarantee that there are exactly 4 digits after the period.
	const prefixLen = len("2006-01-02T15:04:05.000")
	n := len(b)
	t = t.Truncate(time.Millisecond).Add(time.Millisecond / 10)
	b = t.AppendFormat(b, time.RFC3339Nano)
	b = append(b[:n+prefixLen], b[n+prefixLen+1:]...) // drop the 4th digit
	return b
}

// Copied from encoding/json/tables.go.
//
// safeSet holds the value true if the ASCII character with the given array
// position can be represented inside a JSON string without any further
// escaping.
//
// All values are true except for the ASCII control characters (0-31), the
// double quote ("), and the backslash character ("\").
var safeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      true,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      true,
	'=':      true,
	'>':      true,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}

const (
	lowerhex = "0123456789abcdef"
)

func needsEscaping(str string) bool {
	for _, b := range str {
		if !unicode.IsPrint(b) || b == '"' {
			return true
		}
	}

	return false
}

func writeEscapedForOutput(w *handleState, str string, escapeQuotes bool) {
	if !needsEscaping(str) {
		w.appendRawString(str)
		return
	}

	bb := bufPool.Get().(*Buffer)
	bb.Reset()

	bb.Free()

	for _, r := range str {
		if escapeQuotes && r == '"' {
			bb.WriteString(`\"`)
		} else if unicode.IsPrint(r) {
			bb.WriteRune(r)
		} else {
			switch r {
			case '\a':
				bb.WriteString(`\a`)
			case '\b':
				bb.WriteString(`\b`)
			case '\f':
				bb.WriteString(`\f`)
			case '\n':
				bb.WriteString(`\n`)
			case '\r':
				bb.WriteString(`\r`)
			case '\t':
				bb.WriteString("\t")
			case '\v':
				bb.WriteString(`\v`)
			default:
				switch {
				case r < ' ':
					bb.WriteString(`\x`)
					bb.WriteByte(lowerhex[byte(r)>>4])
					bb.WriteByte(lowerhex[byte(r)&0xF])
				case !utf8.ValidRune(r):
					r = 0xFFFD
					fallthrough
				case r < 0x10000:
					bb.WriteString(`\u`)
					for s := 12; s >= 0; s -= 4 {
						bb.WriteByte(lowerhex[r>>uint(s)&0xF])
					}
				default:
					bb.WriteString(`\U`)
					for s := 28; s >= 0; s -= 4 {
						bb.WriteByte(lowerhex[r>>uint(s)&0xF])
					}
				}
			}
		}
	}

	w.appendRawString(bb.String())
}

// testHandler is an implementation of slog.Handler that works
// with the stdlib testing pkg.
type testHandler struct {
	slog.Handler
	t   testing.T
	buf *bytes.Buffer
	mu  *sync.Mutex
}

// Handle implements slog.Handler.
func (b *testHandler) Handle(ctx context.Context, rec slog.Record) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	err := b.Handler.Handle(ctx, rec)
	if err != nil {
		return err
	}

	output, err := io.ReadAll(b.buf)
	if err != nil {
		return err
	}

	// Add calldepth. But it won't be enough, and the internal slog
	// callsite will be printed. See discussion in README.md.
	b.t.Helper()

	// The output comes back with a newline, which we need to
	// trim before feeding to t.Log.
	output = bytes.TrimSuffix(output, []byte("\n"))

	if bytes.ContainsRune(output, '\n') {
		parts := bytes.Split(output, []byte{'\n'})

		for _, x := range parts {
			b.t.Log(string(x))
		}
	} else {
		b.t.Log(string(output))
	}

	return nil
}

// WithAttrs implements slog.Handler.
func (b *testHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &testHandler{
		t:       b.t,
		buf:     b.buf,
		mu:      b.mu,
		Handler: b.Handler.WithAttrs(attrs),
	}
}

// WithGroup implements slog.Handler.
func (b *testHandler) WithGroup(name string) slog.Handler {
	return &testHandler{
		t:       b.t,
		buf:     b.buf,
		mu:      b.mu,
		Handler: b.Handler.WithGroup(name),
	}
}

func NewTest(t testing.T, opts *slog.HandlerOptions) slog.Handler {
	h := &testHandler{
		t:   t,
		buf: new(bytes.Buffer),
		mu:  new(sync.Mutex),
	}

	h.Handler = New(h.buf, opts)

	return h
}
