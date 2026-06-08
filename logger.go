/*
FILE: logger.go

DESCRIPTION:
Public re-export of the Logger interface and typed Field factories. The
interface/type itself lives in internal/gatelog (see documentation there);
here — type aliases and thin wrapper functions.
*/

package gate

import "github.com/tonymontanov/go-gate/v2/internal/gatelog"

// Logger — SDK logging interface. Alias.
type Logger = gatelog.Logger

// Field — typed log field. Alias.
type Field = gatelog.Field

// FieldKind — Field discriminator. Alias.
type FieldKind = gatelog.FieldKind

// FieldKind values.
const (
	FieldKindString = gatelog.FieldKindString
	FieldKindInt    = gatelog.FieldKindInt
	FieldKindFloat  = gatelog.FieldKindFloat
	FieldKindBool   = gatelog.FieldKindBool
	FieldKindError  = gatelog.FieldKindError
)

// NoopLogger returns a no-op logger. Used as the default.
func NoopLogger() Logger { return gatelog.Noop() }

// Str / Int / Float / Bool / Err — Field factories.
func Str(key, value string) Field       { return gatelog.Str(key, value) }
func Int(key string, v int64) Field     { return gatelog.Int(key, v) }
func Float(key string, v float64) Field { return gatelog.Float(key, v) }
func Bool(key string, v bool) Field     { return gatelog.Bool(key, v) }
func Err(err error) Field               { return gatelog.Err(err) }
