package tracing

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/uasneppy/fuku_robot/fuku/utils/metrics"
	"github.com/uasneppy/fuku_robot/fuku/utils/updatememo"
)

var onProcessUpdateCallback atomic.Value

const ContextDataKey = "context"

// UpdateContext returns the update's cancellation, deadline, and trace context.
func UpdateContext(ctx *ext.Context) context.Context {
	if ctx != nil && ctx.Data != nil {
		if updateCtx, ok := ctx.Data[ContextDataKey].(context.Context); ok {
			return updateCtx
		}
	}
	return context.Background()
}

func SetOnProcessUpdateCallback(cb func()) {
	onProcessUpdateCallback.Store(cb)
}

type TracingProcessor struct {
	ext.BaseProcessor
}

func runOnProcessUpdateCallback() {
	if cb, ok := onProcessUpdateCallback.Load().(func()); ok && cb != nil {
		cb()
	}
}

// processingDataKey marks an update this processor is already handling, so a nested call (a
// processor that wraps another) is not counted twice. It is not ContextDataKey: the webhook
// server hands its request context in under that key, and a webhook update is a top-level
// update that needs the metrics and the per-update memo like a polled one (I3).
const processingDataKey = "tracing.processing"

func (tp TracingProcessor) ProcessUpdate(d *ext.Dispatcher, b *gotgbot.Bot, ctx *ext.Context) (err error) {
	if ctx == nil {
		return tp.BaseProcessor.ProcessUpdate(d, b, ctx)
	}
	runOnProcessUpdateCallback()

	if ctx.Data == nil {
		ctx.Data = make(map[string]any)
	}
	if _, nested := ctx.Data[processingDataKey]; nested {
		return tp.BaseProcessor.ProcessUpdate(d, b, ctx)
	}
	ctx.Data[processingDataKey] = true

	start := time.Now()
	var span trace.Span
	if _, exists := ctx.Data[ContextDataKey].(context.Context); !exists {
		// The 30-second deadline is load-bearing: tracing.UpdateContext hands this
		// context to context-aware repositories, and
		// docs/src/content/docs/architecture/caching.md documents that polling and
		// webhook updates carry it. It is created unconditionally, not only when
		// tracing is on. A webhook update already carries the server's own bounded,
		// traced context, which is kept.
		baseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var traceCtx context.Context
		traceCtx, span = StartSpan(baseCtx, "dispatcher.processUpdate")
		ctx.Data[ContextDataKey] = traceCtx
	}
	defer func() {
		metrics.UpdatesProcessed.Inc()
		metrics.UpdateDuration.Observe(time.Since(start).Seconds())
		if span != nil {
			if err != nil {
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}
	}()

	ctx.Data[updatememo.DataKey] = updatememo.New()

	err = tp.BaseProcessor.ProcessUpdate(d, b, ctx)
	return err
}
