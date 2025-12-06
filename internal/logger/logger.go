package logger

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// ANSI颜色码
const (
	colorReset    = "\033[0m"
	colorRed      = "\033[31m"
	colorGreen    = "\033[32m"
	colorYellow   = "\033[33m"
	colorBlue     = "\033[34m"
	colorMagenta  = "\033[35m"
	colorCyan     = "\033[36m"
	colorGray     = "\033[90m"
	colorRedBold  = "\033[1;31m"
	colorTime     = "\033[36m" // Cyan (青色) 用于时间
	colorThreadID = "\033[35m" // Magenta (洋红) 用于线程ID
	colorCaller   = "\033[90m" // Gray (灰色) 用于调用者
)

// RedirectStdLog 重定向标准库的 log 输出到 zap
func RedirectStdLog() {
	if Logger == nil {
		return
	}

	// 使用 zap 的 RedirectStdLogAt 功能
	// 这会替换标准库的 log 输出，将标准库的日志重定向到我们的 zap logger
	// 这样 canal 库使用标准库 log 的输出就会被我们的 logger 捕获
	undo, err := zap.RedirectStdLogAt(Logger, zapcore.InfoLevel)
	if err == nil {
		// 保存 undo 函数以便后续恢复（如果需要）
		_ = undo
	}
}

var Logger *zap.Logger
var Sugar *zap.SugaredLogger

// callerEncoderPool 复用 callerEncoder 的池
var callerEncoderPool = sync.Pool{
	New: func() interface{} {
		return &callerEncoder{
			buf: buffer.NewPool().Get(),
		}
	},
}

// callerEncoder 简单的辅助编码器，用于调用 zap 的 ShortCallerEncoder
type callerEncoder struct {
	buf *buffer.Buffer
}

func (e *callerEncoder) AppendBool(val bool)             { e.buf.AppendBool(val) }
func (e *callerEncoder) AppendByteString(val []byte)     { e.buf.AppendString(string(val)) }
func (e *callerEncoder) AppendComplex128(val complex128) { e.buf.AppendString(fmt.Sprintf("%g", val)) }
func (e *callerEncoder) AppendComplex64(val complex64)   { e.AppendComplex128(complex128(val)) }
func (e *callerEncoder) AppendFloat64(val float64)       { e.buf.AppendFloat(val, 64) }
func (e *callerEncoder) AppendFloat32(val float32)       { e.buf.AppendFloat(float64(val), 32) }
func (e *callerEncoder) AppendInt(val int)               { e.buf.AppendInt(int64(val)) }
func (e *callerEncoder) AppendInt64(val int64)           { e.buf.AppendInt(val) }
func (e *callerEncoder) AppendInt32(val int32)           { e.buf.AppendInt(int64(val)) }
func (e *callerEncoder) AppendInt16(val int16)           { e.buf.AppendInt(int64(val)) }
func (e *callerEncoder) AppendInt8(val int8)             { e.buf.AppendInt(int64(val)) }
func (e *callerEncoder) AppendString(val string)         { e.buf.AppendString(val) }
func (e *callerEncoder) AppendUint(val uint)             { e.buf.AppendUint(uint64(val)) }
func (e *callerEncoder) AppendUint64(val uint64)         { e.buf.AppendUint(val) }
func (e *callerEncoder) AppendUint32(val uint32)         { e.buf.AppendUint(uint64(val)) }
func (e *callerEncoder) AppendUint16(val uint16)         { e.buf.AppendUint(uint64(val)) }
func (e *callerEncoder) AppendUint8(val uint8)           { e.buf.AppendUint(uint64(val)) }
func (e *callerEncoder) AppendUintptr(val uintptr)       { e.buf.AppendUint(uint64(val)) }

// goroutineIDBufferPool 复用 stack buffer，减少内存分配
var goroutineIDBufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 64)
		return &b
	},
}

// getGoroutineID 获取当前 goroutine ID
// 注意：runtime.Stack 调用较慢，但这是获取 goroutine ID 的标准方式
// 已优化：使用 sync.Pool 复用 buffer，减少内存分配
func getGoroutineID() uint64 {
	bufPtr := goroutineIDBufferPool.Get().(*[]byte)
	defer goroutineIDBufferPool.Put(bufPtr)

	b := *bufPtr
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

// formatLevel 格式化日志级别，返回带颜色的字符串
func formatLevel(level zapcore.Level, enableColor bool) string {
	var levelStr string
	var color string

	switch level {
	case zapcore.DebugLevel:
		levelStr = "[D]"
		if enableColor {
			color = colorGray
		}
	case zapcore.InfoLevel:
		levelStr = "[I]"
		if enableColor {
			color = colorGreen
		}
	case zapcore.WarnLevel:
		levelStr = "[W]"
		if enableColor {
			color = colorYellow
		}
	case zapcore.ErrorLevel:
		levelStr = "[E]"
		if enableColor {
			color = colorRed
		}
	case zapcore.FatalLevel, zapcore.DPanicLevel, zapcore.PanicLevel:
		levelStr = "[F]"
		if enableColor {
			color = colorRedBold
		}
	default:
		levelStr = "[I]"
		if enableColor {
			color = colorGreen
		}
	}

	if enableColor && color != "" {
		return color + levelStr + colorReset
	}
	return levelStr
}

// appendTime 直接将时间格式化追加到 buffer（优化：消除 fmt.Sprintf）
// 格式：mmdd hh:mm:ss.uuuuuu
func appendTime(buf *buffer.Buffer, t time.Time, enableColor bool) {
	month := int(t.Month())
	day := t.Day()
	hour := t.Hour()
	minute := t.Minute()
	second := t.Second()
	microsecond := t.Nanosecond() / 1000

	if enableColor {
		buf.AppendString(colorTime)
	}

	// mmdd - 手动格式化，避免 fmt.Sprintf
	if month < 10 {
		buf.AppendByte('0')
	}
	buf.AppendInt(int64(month))
	if day < 10 {
		buf.AppendByte('0')
	}
	buf.AppendInt(int64(day))
	buf.AppendByte(' ')

	// hh:mm:ss - 手动格式化
	if hour < 10 {
		buf.AppendByte('0')
	}
	buf.AppendInt(int64(hour))
	buf.AppendByte(':')
	if minute < 10 {
		buf.AppendByte('0')
	}
	buf.AppendInt(int64(minute))
	buf.AppendByte(':')
	if second < 10 {
		buf.AppendByte('0')
	}
	buf.AppendInt(int64(second))
	buf.AppendByte('.')

	// uuuuuu (6位微秒) - 手动格式化
	// 从高位到低位写入
	digits := [6]byte{}
	ms := microsecond
	for i := 5; i >= 0; i-- {
		digits[i] = byte('0' + ms%10)
		ms /= 10
	}
	for i := 0; i < 6; i++ {
		buf.AppendByte(digits[i])
	}

	if enableColor {
		buf.AppendString(colorReset)
	}
}

// customLevelEncoder 自定义级别编码器（用于zap配置，但实际不使用）
func customLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch l {
	case zapcore.DebugLevel:
		enc.AppendString("[D]")
	case zapcore.InfoLevel:
		enc.AppendString("[I]")
	case zapcore.WarnLevel:
		enc.AppendString("[W]")
	case zapcore.ErrorLevel:
		enc.AppendString("[E]")
	case zapcore.FatalLevel, zapcore.DPanicLevel, zapcore.PanicLevel:
		enc.AppendString("[F]")
	default:
		enc.AppendString("[I]")
	}
}

// customTimeEncoder 自定义时间编码器（用于zap配置，但实际不使用）
// 注意：在实际的 EncodeEntry 中，我们直接使用 appendTime 方法，这个方法只是占位符
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	// 创建一个临时 buffer 来格式化时间
	buf := buffer.NewPool().Get()
	defer buf.Free()
	appendTime(buf, t, false)
	enc.AppendString(buf.String())
}

// customEncoder 自定义编码器实现
type customEncoder struct {
	*zapcore.EncoderConfig
	buf               *buffer.Buffer
	fieldSeparator    string
	enableColor       bool
	enableGoroutineID bool // 是否启用 goroutine ID 打印
}

// isTerminal 检查文件描述符是否是终端
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// newCustomEncoder 创建自定义编码器
func newCustomEncoder(cfg zapcore.EncoderConfig, enableColor, enableGoroutineID bool) *customEncoder {
	return &customEncoder{
		EncoderConfig:     &cfg,
		buf:               buffer.NewPool().Get(),
		fieldSeparator:    " ",
		enableColor:       enableColor,
		enableGoroutineID: enableGoroutineID,
	}
}

// Clone 克隆编码器
func (e *customEncoder) Clone() zapcore.Encoder {
	clone := &customEncoder{
		EncoderConfig:     e.EncoderConfig,
		buf:               buffer.NewPool().Get(),
		fieldSeparator:    e.fieldSeparator,
		enableColor:       e.enableColor,
		enableGoroutineID: e.enableGoroutineID,
	}
	return clone
}

// addField 添加字段到缓冲区（优化：避免 fmt.Sprintf 和 String() 调用）
func (e *customEncoder) addField(key, value string) {
	if e.buf.Len() > 0 {
		// 检查最后一个字符是否为空格（使用 buffer 方法而不是 String()）
		e.buf.AppendString(e.fieldSeparator)
	}
	e.buf.AppendString(key)
	e.buf.AppendString("=")
	e.buf.AppendString(value)
}

// AddArray 添加数组
func (e *customEncoder) AddArray(key string, marshaler zapcore.ArrayMarshaler) error {
	arrEnc := &arrayEncoder{
		parent: e,
		buf:    buffer.NewPool().Get(),
		first:  true,
	}
	err := marshaler.MarshalLogArray(arrEnc)
	if err != nil {
		arrEnc.buf.Free()
		return err
	}
	result := arrEnc.buf.String()
	arrEnc.buf.Free()
	e.addField(key, result)
	return nil
}

// AddObject 添加对象
func (e *customEncoder) AddObject(key string, marshaler zapcore.ObjectMarshaler) error {
	objEnc := &objectEncoder{
		parent: e,
		buf:    buffer.NewPool().Get(),
		first:  true,
	}
	err := marshaler.MarshalLogObject(objEnc)
	if err != nil {
		objEnc.buf.Free()
		return err
	}
	result := objEnc.buf.String()
	objEnc.buf.Free()
	e.addField(key, result)
	return nil
}

// arrayEncoder 数组编码器辅助类型
type arrayEncoder struct {
	parent *customEncoder
	buf    *buffer.Buffer
	first  bool
}

func (a *arrayEncoder) AppendBool(val bool) { a.appendSeparator(); a.buf.AppendBool(val) }
func (a *arrayEncoder) AppendByteString(val []byte) {
	a.appendSeparator()
	a.buf.AppendString(string(val))
}
func (a *arrayEncoder) AppendComplex128(val complex128) {
	a.appendSeparator()
	a.buf.AppendString(fmt.Sprintf("%g", val))
}
func (a *arrayEncoder) AppendComplex64(val complex64) { a.AppendComplex128(complex128(val)) }
func (a *arrayEncoder) AppendFloat64(val float64)     { a.appendSeparator(); a.buf.AppendFloat(val, 64) }
func (a *arrayEncoder) AppendFloat32(val float32) {
	a.appendSeparator()
	a.buf.AppendFloat(float64(val), 32)
}
func (a *arrayEncoder) AppendInt(val int)         { a.appendSeparator(); a.buf.AppendInt(int64(val)) }
func (a *arrayEncoder) AppendInt64(val int64)     { a.appendSeparator(); a.buf.AppendInt(val) }
func (a *arrayEncoder) AppendInt32(val int32)     { a.appendSeparator(); a.buf.AppendInt(int64(val)) }
func (a *arrayEncoder) AppendInt16(val int16)     { a.appendSeparator(); a.buf.AppendInt(int64(val)) }
func (a *arrayEncoder) AppendInt8(val int8)       { a.appendSeparator(); a.buf.AppendInt(int64(val)) }
func (a *arrayEncoder) AppendString(val string)   { a.appendSeparator(); a.buf.AppendString(val) }
func (a *arrayEncoder) AppendUint(val uint)       { a.appendSeparator(); a.buf.AppendUint(uint64(val)) }
func (a *arrayEncoder) AppendUint64(val uint64)   { a.appendSeparator(); a.buf.AppendUint(val) }
func (a *arrayEncoder) AppendUint32(val uint32)   { a.appendSeparator(); a.buf.AppendUint(uint64(val)) }
func (a *arrayEncoder) AppendUint16(val uint16)   { a.appendSeparator(); a.buf.AppendUint(uint64(val)) }
func (a *arrayEncoder) AppendUint8(val uint8)     { a.appendSeparator(); a.buf.AppendUint(uint64(val)) }
func (a *arrayEncoder) AppendUintptr(val uintptr) { a.appendSeparator(); a.buf.AppendUint(uint64(val)) }
func (a *arrayEncoder) AppendDuration(val time.Duration) {
	a.appendSeparator()
	a.buf.AppendString(val.String())
}
func (a *arrayEncoder) AppendTime(val time.Time) {
	a.appendSeparator()
	a.buf.AppendString(val.Format(time.RFC3339))
}
func (a *arrayEncoder) AppendArray(marshaler zapcore.ArrayMarshaler) error {
	a.appendSeparator()
	enc := &arrayEncoder{parent: a.parent, buf: buffer.NewPool().Get(), first: true}
	err := marshaler.MarshalLogArray(enc)
	if err == nil {
		a.buf.AppendString("[")
		a.buf.AppendBytes(enc.buf.Bytes()) // 直接追加字节，避免 String()
		a.buf.AppendString("]")
	}
	enc.buf.Free()
	return err
}
func (a *arrayEncoder) AppendObject(marshaler zapcore.ObjectMarshaler) error {
	a.appendSeparator()
	enc := &objectEncoder{parent: a.parent, buf: buffer.NewPool().Get(), first: true}
	err := marshaler.MarshalLogObject(enc)
	if err == nil {
		a.buf.AppendString("{")
		a.buf.AppendBytes(enc.buf.Bytes()) // 直接追加字节，避免 String()
		a.buf.AppendString("}")
	}
	enc.buf.Free()
	return err
}
func (a *arrayEncoder) AppendReflected(val interface{}) error {
	a.appendSeparator()
	a.buf.AppendString(fmt.Sprintf("%v", val))
	return nil
}
func (a *arrayEncoder) appendSeparator() {
	if !a.first {
		a.buf.AppendString(",")
	}
	a.first = false
}

// objectEncoder 对象编码器辅助类型
type objectEncoder struct {
	parent *customEncoder
	buf    *buffer.Buffer
	first  bool
}

func (o *objectEncoder) AddBool(key string, val bool) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendBool(val)
}
func (o *objectEncoder) AddByteString(key string, val []byte) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendString(string(val))
}
func (o *objectEncoder) AddComplex128(key string, val complex128) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendString(fmt.Sprintf("%g", val)) // 复数需要使用 fmt，无法避免
}
func (o *objectEncoder) AddComplex64(key string, val complex64) {
	o.AddComplex128(key, complex128(val))
}
func (o *objectEncoder) AddFloat64(key string, val float64) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendFloat(val, 64)
}
func (o *objectEncoder) AddFloat32(key string, val float32) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendFloat(float64(val), 32)
}
func (o *objectEncoder) AddInt(key string, val int) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendInt(int64(val))
}
func (o *objectEncoder) AddInt64(key string, val int64) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendInt(val)
}
func (o *objectEncoder) AddInt32(key string, val int32) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendInt(int64(val))
}
func (o *objectEncoder) AddInt16(key string, val int16) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendInt(int64(val))
}
func (o *objectEncoder) AddInt8(key string, val int8) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendInt(int64(val))
}
func (o *objectEncoder) AddString(key, val string) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendString(val)
}
func (o *objectEncoder) AddUint(key string, val uint) {
	o.addUint(key, uint64(val))
}
func (o *objectEncoder) AddUint64(key string, val uint64) {
	o.addUint(key, val)
}
func (o *objectEncoder) AddUint32(key string, val uint32) {
	o.addUint(key, uint64(val))
}
func (o *objectEncoder) AddUint16(key string, val uint16) {
	o.addUint(key, uint64(val))
}
func (o *objectEncoder) AddUint8(key string, val uint8) {
	o.addUint(key, uint64(val))
}
func (o *objectEncoder) AddUintptr(key string, val uintptr) {
	o.addUint(key, uint64(val))
}

// addUint 辅助方法，统一处理所有 uint 类型
func (o *objectEncoder) addUint(key string, val uint64) {
	o.appendSeparator()
	o.buf.AppendString(fmt.Sprintf("%s=%d", key, val))
}
func (o *objectEncoder) AddBinary(key string, val []byte) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	// 十六进制编码，需要使用 fmt，但可以减少字符串拼接
	for _, b := range val {
		o.buf.AppendByte(hexDigits[b>>4])
		o.buf.AppendByte(hexDigits[b&0x0f])
	}
}

// hexDigits 十六进制字符表
var hexDigits = []byte("0123456789abcdef")

func (o *objectEncoder) AddArray(key string, marshaler zapcore.ArrayMarshaler) error {
	o.appendSeparator()
	enc := &arrayEncoder{parent: o.parent, buf: buffer.NewPool().Get(), first: true}
	err := marshaler.MarshalLogArray(enc)
	if err == nil {
		o.buf.AppendString(key)
		o.buf.AppendString("=[")
		o.buf.AppendBytes(enc.buf.Bytes()) // 直接追加字节，避免 String()
		o.buf.AppendString("]")
	}
	enc.buf.Free()
	return err
}
func (o *objectEncoder) AddObject(key string, marshaler zapcore.ObjectMarshaler) error {
	o.appendSeparator()
	enc := &objectEncoder{parent: o.parent, buf: buffer.NewPool().Get(), first: true}
	err := marshaler.MarshalLogObject(enc)
	if err == nil {
		o.buf.AppendString(key)
		o.buf.AppendString("={")
		o.buf.AppendBytes(enc.buf.Bytes()) // 直接追加字节，避免 String()
		o.buf.AppendString("}")
	}
	enc.buf.Free()
	return err
}
func (o *objectEncoder) AddReflected(key string, val interface{}) error {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendString(fmt.Sprintf("%v", val)) // 反射值必须使用 fmt
	return nil
}
func (o *objectEncoder) AddTime(key string, val time.Time) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendTime(val, time.RFC3339)
}
func (o *objectEncoder) AddDuration(key string, val time.Duration) {
	o.appendSeparator()
	o.buf.AppendString(key)
	o.buf.AppendString("=")
	o.buf.AppendString(val.String())
}
func (o *objectEncoder) OpenNamespace(key string) {}
func (o *objectEncoder) appendSeparator() {
	if !o.first {
		o.buf.AppendString(" ")
	}
	o.first = false
}

// AddBinary 添加二进制（优化：避免 fmt.Sprintf）
func (e *customEncoder) AddBinary(key string, value []byte) {
	// 使用 buffer 直接构建十六进制字符串，避免 fmt.Sprintf
	tempBuf := buffer.NewPool().Get()
	defer tempBuf.Free()
	for _, b := range value {
		tempBuf.AppendByte(hexDigits[b>>4])
		tempBuf.AppendByte(hexDigits[b&0x0f])
	}
	e.addField(key, tempBuf.String())
}

// AddByteString 添加字节字符串
func (e *customEncoder) AddByteString(key string, value []byte) {
	e.addField(key, string(value))
}

// AddBool 添加布尔值
func (e *customEncoder) AddBool(key string, value bool) {
	e.addField(key, strconv.FormatBool(value))
}

// AddComplex128 添加复数
func (e *customEncoder) AddComplex128(key string, value complex128) {
	e.addField(key, fmt.Sprintf("%g", value))
}

// AddComplex64 添加复数
func (e *customEncoder) AddComplex64(key string, value complex64) {
	e.AddComplex128(key, complex128(value))
}

// AddDuration 添加时长
func (e *customEncoder) AddDuration(key string, value time.Duration) {
	e.addField(key, value.String())
}

// AddFloat64 添加浮点数
func (e *customEncoder) AddFloat64(key string, value float64) {
	e.addField(key, fmt.Sprintf("%g", value))
}

// AddFloat32 添加浮点数
func (e *customEncoder) AddFloat32(key string, value float32) {
	e.AddFloat64(key, float64(value))
}

// AddInt 添加整数
func (e *customEncoder) AddInt(key string, value int) {
	e.AddInt64(key, int64(value))
}

// AddInt64 添加整数
func (e *customEncoder) AddInt64(key string, value int64) {
	e.addField(key, strconv.FormatInt(value, 10))
}

// AddInt32 添加整数
func (e *customEncoder) AddInt32(key string, value int32) {
	e.AddInt64(key, int64(value))
}

// AddInt16 添加整数
func (e *customEncoder) AddInt16(key string, value int16) {
	e.AddInt64(key, int64(value))
}

// AddInt8 添加整数
func (e *customEncoder) AddInt8(key string, value int8) {
	e.AddInt64(key, int64(value))
}

// AddString 添加字符串
func (e *customEncoder) AddString(key, value string) {
	e.addField(key, value)
}

// AddTime 添加时间
func (e *customEncoder) AddTime(key string, value time.Time) {
	e.addField(key, value.Format(time.RFC3339))
}

// AddUint 添加无符号整数
func (e *customEncoder) AddUint(key string, value uint) {
	e.AddUint64(key, uint64(value))
}

// AddUint64 添加无符号整数
func (e *customEncoder) AddUint64(key string, value uint64) {
	// strconv.FormatUint 已经是最优化的实现
	e.addField(key, strconv.FormatUint(value, 10))
}

// AddUint32 添加无符号整数
func (e *customEncoder) AddUint32(key string, value uint32) {
	e.AddUint64(key, uint64(value))
}

// AddUint16 添加无符号整数
func (e *customEncoder) AddUint16(key string, value uint16) {
	e.AddUint64(key, uint64(value))
}

// AddUint8 添加无符号整数
func (e *customEncoder) AddUint8(key string, value uint8) {
	e.AddUint64(key, uint64(value))
}

// AddUintptr 添加无符号指针
func (e *customEncoder) AddUintptr(key string, value uintptr) {
	e.AddUint64(key, uint64(value))
}

// AddReflected 添加反射值
func (e *customEncoder) AddReflected(key string, value interface{}) error {
	e.addField(key, fmt.Sprintf("%v", value))
	return nil
}

// OpenNamespace 打开命名空间
func (e *customEncoder) OpenNamespace(key string) {
	// 自定义格式中不需要命名空间
}

// EncodeEntry 编码日志条目
func (e *customEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	// 使用新的缓冲区
	final := buffer.NewPool().Get()

	// 格式：[L] mmdd hh:mm:ss.uuuuuu threadid file:line] msg...

	// 1. 级别 ([L])，直接格式化
	final.AppendString(formatLevel(ent.Level, e.enableColor))
	final.AppendString(" ") // 级别和时间之间的空格

	// 2. 时间 (mmdd hh:mm:ss.uuuuuu)，直接追加到 buffer（优化：消除 fmt.Sprintf）
	appendTime(final, ent.Time, e.enableColor)
	final.AppendString(" ")

	// 3. goroutine ID (threadid)，可选配置（仅在启用时打印）
	if e.enableGoroutineID {
		goroutineID := getGoroutineID()
		if e.enableColor {
			final.AppendString(colorThreadID)
		}
		// 手动格式化 3 位数字（避免 fmt.Sprintf）
		if goroutineID < 10 {
			final.AppendString("00")
			final.AppendUint(goroutineID)
		} else if goroutineID < 100 {
			final.AppendString("0")
			final.AppendUint(goroutineID)
		} else {
			final.AppendUint(goroutineID)
		}
		if e.enableColor {
			final.AppendString(colorReset)
		}
		final.AppendString(" ")
	}

	// 4. 调用者 (file:line)，带颜色（优化：复用 callerEncoder，直接追加 bytes）
	if ent.Caller.Defined {
		// 从池中获取 callerEncoder
		callerEnc := callerEncoderPool.Get().(*callerEncoder)
		callerEnc.buf.Reset()

		zapcore.ShortCallerEncoder(ent.Caller, callerEnc)

		// 直接追加 bytes，避免 String() 分配
		if e.enableColor {
			final.AppendString(colorCaller)
		}
		final.AppendBytes(callerEnc.buf.Bytes())
		if e.enableColor {
			final.AppendString(colorReset)
		}

		// 归还到池中
		callerEncoderPool.Put(callerEnc)
	} else {
		if e.enableColor {
			final.AppendString(colorCaller)
		}
		final.AppendString("unknown:0")
		if e.enableColor {
			final.AppendString(colorReset)
		}
	}
	final.AppendString(" | ")

	// 5. 消息 (msg...)
	final.AppendString(ent.Message)

	// 6. 字段（优化：直接追加 bytes，避免 String() 分配）
	if len(fields) > 0 {
		// 临时重置buf用于收集字段
		e.buf.Reset()
		for _, field := range fields {
			field.AddTo(e)
		}
		if e.buf.Len() > 0 {
			final.AppendString(" ")
			// 直接追加 buffer 的字节，避免 String() 分配
			final.AppendBytes(e.buf.Bytes())
		}
	}

	// 7. 错误堆栈信息
	if ent.Stack != "" {
		final.AppendString("\n")
		final.AppendString(ent.Stack)
	}

	final.AppendString("\n")
	return final, nil
}

// InitLogger 初始化日志
// enableGoroutineID: 是否在日志中打印 goroutine ID（会影响性能，因为需要调用 runtime.Stack）
func InitLogger(level, format, output, file string, maxSize, maxBackups, maxAge int, enableGoroutineID bool) error {
	// 解析日志级别
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	// 配置输出
	var writeSyncer zapcore.WriteSyncer
	var enableColor bool
	if output == "file" && file != "" {
		// 文件输出，支持日志轮转，不启用颜色
		enableColor = false
		lumberjackLogger := &lumberjack.Logger{
			Filename:   file,
			MaxSize:    maxSize, // MB
			MaxBackups: maxBackups,
			MaxAge:     maxAge, // days
			Compress:   true,
		}
		writeSyncer = zapcore.AddSync(lumberjackLogger)
	} else {
		// 标准输出，检查是否是终端来决定是否启用颜色
		enableColor = isTerminal(os.Stdout)
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// 配置编码器
	var encoder zapcore.Encoder
	if format == "json" {
		encoderConfig := zap.NewProductionEncoderConfig()
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		// 使用自定义格式：[L] mmdd hh:mm:ss.uuuuuu threadid file:line] msg...
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    customLevelEncoder,
			EncodeTime:     customTimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		encoder = newCustomEncoder(encoderConfig, enableColor, enableGoroutineID)
	}

	// 创建核心
	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)

	// 创建 logger，跳过一层调用（因为我们的封装函数）
	// 这样日志会显示实际调用者的位置，而不是封装函数的位置
	Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()

	// 重定向标准库的 log 输出到 zap（用于 canal 等库的日志）
	RedirectStdLog()

	return nil
}

// Sync 同步日志缓冲区
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}

// 以下函数提供简化的调用方式，直接使用 logger.Infow() 而不是 logger.Sugar.Infow()

// Debug 记录 Debug 级别日志
func Debug(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Debugw(msg, keysAndValues...)
	}
}

// Info 记录 Info 级别日志
func Info(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Infow(msg, keysAndValues...)
	}
}

// Infof 记录 Info 级别日志（格式化字符串）
func Infof(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Infof(template, args...)
	}
}

// Warn 记录 Warn 级别日志
func Warn(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Warnw(msg, keysAndValues...)
	}
}

// Warnf 记录 Warn 级别日志（格式化字符串）
func Warnf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Warnf(template, args...)
	}
}

// Error 记录 Error 级别日志
func Error(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Errorw(msg, keysAndValues...)
	}
}

// Errorf 记录 Error 级别日志（格式化字符串）
func Errorf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Errorf(template, args...)
	}
}

// Fatal 记录 Fatal 级别日志并退出
func Fatal(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalw(msg, keysAndValues...)
	}
}

// Fatalf 记录 Fatal 级别日志并退出（格式化字符串）
func Fatalf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalf(template, args...)
	}
}

// Debugw 记录 Debug 级别日志（with fields）
func Debugw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Debugw(msg, keysAndValues...)
	}
}

// Infow 记录 Info 级别日志（with fields）
func Infow(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Infow(msg, keysAndValues...)
	}
}

// Warnw 记录 Warn 级别日志（with fields）
func Warnw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Warnw(msg, keysAndValues...)
	}
}

// Errorw 记录 Error 级别日志（with fields）
func Errorw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Errorw(msg, keysAndValues...)
	}
}

// Fatalw 记录 Fatal 级别日志并退出（with fields）
func Fatalw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalw(msg, keysAndValues...)
	}
}
