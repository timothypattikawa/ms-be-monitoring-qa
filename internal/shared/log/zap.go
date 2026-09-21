package log

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapio"
)

type (
	Logger interface {
		WithError(err any) Logger
		WithField(key string, val any) Logger
		WithFields(fields map[string]any) Logger
		WithMap(fields any) Logger
		WithContext(ctx context.Context) Logger

		Debug(message string)
		Debugf(message string, args ...any)
		Error(message string)
		Errorf(message string, args ...any)
		Fatal(message string)
		Fatalf(message string, args ...any)
		Info(message string)
		Infof(message string, args ...any)
		Warn(message string)
		Warnf(message string, args ...any)

		GetWriter() io.Writer
		Close() error
	}

	zapLogger struct {
		instance   *zap.Logger
		writer     *zapio.Writer
		onceWriter *sync.Once
		withCaller bool
	}
)

func (z *zapLogger) WithError(err any) Logger {
	switch err.(type) {
	case error:
		return &zapLogger{instance: z.Get().With(zap.String("error", err.(error).Error()))}
	default:
		return &zapLogger{instance: z.Get().With(zap.Reflect("error", err))}
	}
}

func (z *zapLogger) WithField(key string, val any) Logger {
	return &zapLogger{instance: z.Get().With(zap.Reflect(key, val))}
}

func (z *zapLogger) WithFields(fields map[string]any) Logger {
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Reflect(k, v))
	}
	return &zapLogger{instance: z.Get().With(zapFields...)}
}

func (z *zapLogger) WithMap(fields any) Logger {
	kind := reflect.TypeOf(fields).Kind()
	maps := make(map[string]any)

	switch kind {
	case reflect.Slice, reflect.Array:
		iterator := reflect.ValueOf(fields).MapRange()
		for iterator.Next() {
			maps[iterator.Key().String()] = iterator.Value()
		}
		return z.WithFields(maps)
	case reflect.Struct:
		vo := reflect.ValueOf(fields)
		for i := 0; i < vo.NumField(); i++ {
			field := vo.Type().Field(i)
			key := field.Tag.Get("json")
			if key == "" {
				key = field.Name
			}
			maps[key] = vo.Field(i).Interface()
		}
		return z.WithFields(maps)
	case reflect.Pointer:
		vo := reflect.ValueOf(fields).Elem()
		for i := 0; i < vo.NumField(); i++ {
			field := vo.Type().Field(i)
			key := field.Tag.Get("json")
			if key == "" {
				key = field.Name
			}
			maps[key] = vo.Field(i).Interface()
		}
		return z.WithFields(maps)
	default:
		return z.WithField("map", fields)
	}
}

func (z *zapLogger) WithContext(ctx context.Context) Logger {
	fields := make([]zap.Field, 0)

	if requestId, ok := ctx.Value("X-Request-ID").(string); ok {
		fields = append(fields, zap.String("request_id", requestId))
	}

	withCaller, ok := ctx.Value("WithCaller").(bool)
	if !ok {
		withCaller = true
	}

	return &zapLogger{
		instance:   z.Get().With(fields...),
		writer:     nil,
		onceWriter: nil,
		withCaller: withCaller,
	}
}

func (z *zapLogger) Debug(message string) {
	z.Get().With(z.GetMandatoryFields()...).Debug(message)
}

func (z *zapLogger) Debugf(message string, args ...any) {
	z.Debug(fmt.Sprintf(message, args...))
}

func (z *zapLogger) Error(message string) {
	z.Get().With(z.GetMandatoryFields()...).Error(message)
}

func (z *zapLogger) Errorf(message string, args ...any) {
	z.Error(fmt.Sprintf(message, args...))
}

func (z *zapLogger) Fatal(message string) {
	z.Get().With(z.GetMandatoryFields()...).Fatal(message)
}

func (z *zapLogger) Fatalf(message string, args ...any) {
	z.Fatal(fmt.Sprintf(message, args...))
}

func (z *zapLogger) Info(message string) {
	z.Get().With(z.GetMandatoryFields()...).Info(message)
}

func (z *zapLogger) Infof(message string, args ...any) {
	z.Info(fmt.Sprintf(message, args...))
}

func (z *zapLogger) Warn(message string) {
	z.Get().With(z.GetMandatoryFields()...).Warn(message)
}

func (z *zapLogger) Warnf(message string, args ...any) {
	z.Warn(fmt.Sprintf(message, args...))
}

func (z *zapLogger) GetWriter() io.Writer {
	z.onceWriter.Do(
		func() {
			z.writer = &zapio.Writer{Log: z.instance, Level: zapcore.InfoLevel}
		},
	)
	return z.writer
}

func (z *zapLogger) Close() error {
	if z.writer != nil {
		_ = z.writer.Close()
	}
	return z.instance.Sync()
}

func (z *zapLogger) Get() *zap.Logger {
	return z.instance
}

func (z *zapLogger) GetMandatoryFields() []zap.Field {
	var fields []zap.Field
	if z.withCaller {
		fields = append(fields, zap.String("caller", utils.GetSource(3)))
	}

	return fields
}

func NewLogger(o *Option) (Logger, error) {
	var (
		logger = &zapLogger{onceWriter: new(sync.Once)}
		cfg    = zap.NewProductionConfig()
		err    error
	)

	if o == nil {
		o = NewDefaultOption()
	}

	cfg.DisableCaller = true
	cfg.OutputPaths = append(cfg.OutputPaths, o.OutputPath)
	cfg.Level, err = zap.ParseAtomicLevel(o.Level)
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.DisableStacktrace = true
	if err != nil {
		cfg.Level = zap.NewAtomicLevel() // set Logger Level to INFO level
	}

	logger.instance, err = cfg.Build()

	return logger, err
}
