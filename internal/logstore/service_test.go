// The library github.com/benbjohnson/clock fails when race is enabled
// https://github.com/benbjohnson/clock/issues/44
//go:build !race

package logstore_test

import (
	"context"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/zitadel/zitadel/internal/logstore"

	"github.com/zitadel/zitadel/internal/repository/instance"

	"github.com/benbjohnson/clock"

	emittermock "github.com/zitadel/zitadel/internal/logstore/emitters/mock"
	quotaqueriermock "github.com/zitadel/zitadel/internal/logstore/quotaqueriers/mock"
	"github.com/zitadel/zitadel/internal/query"
)

const (
	tick  = time.Second
	ticks = 60
)

func TestService(t *testing.T) {
	// tests should run on a single thread
	// important for deterministic results
	beforeProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(beforeProcs)

	type args struct {
		mainSink      *logstore.EmitterConfig
		secondarySink *logstore.EmitterConfig
		quota         query.Quota
	}
	type wantSink struct {
		bulks []int
		len   int
	}
	type want struct {
		enabled       bool
		remaining     *uint64
		mainSink      wantSink
		secondarySink wantSink
	}
	tests := []struct {
		name string
		args args
		want want
	}{{
		name: "max and min debouncing works",
		args: args{
			mainSink: emitterConfig(withDebouncerConfig(&logstore.DebouncerConfig{
				MinFrequency: 1 * time.Minute,
				MaxBulkSize:  60,
			})),
			secondarySink: emitterConfig(),
			quota:         quotaConfig(),
		},
		want: want{
			enabled:   true,
			remaining: nil,
			mainSink: wantSink{
				bulks: repeat(60, 1),
				len:   60,
			},
			secondarySink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
		},
	}, {
		name: "mixed debouncing works",
		args: args{
			mainSink: emitterConfig(withDebouncerConfig(&logstore.DebouncerConfig{
				MinFrequency: 0,
				MaxBulkSize:  6,
			})),
			secondarySink: emitterConfig(withDebouncerConfig(&logstore.DebouncerConfig{
				MinFrequency: 10 * time.Second,
				MaxBulkSize:  0,
			})),
			quota: quotaConfig(),
		},
		want: want{
			enabled:   true,
			remaining: nil,
			mainSink: wantSink{
				bulks: repeat(6, 10),
				len:   60,
			},
			secondarySink: wantSink{
				bulks: repeat(10, 6),
				len:   60,
			},
		},
	}, {
		name: "when disabling main sink, secondary sink still works",
		args: args{
			mainSink:      emitterConfig(withDisabled()),
			secondarySink: emitterConfig(),
			quota:         quotaConfig(),
		},
		want: want{
			enabled:   true,
			remaining: nil,
			mainSink: wantSink{
				bulks: repeat(99, 0),
				len:   0,
			},
			secondarySink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
		},
	}, {
		name: "when all sink are disabled, the service is disabled",
		args: args{
			mainSink:      emitterConfig(withDisabled()),
			secondarySink: emitterConfig(withDisabled()),
			quota:         quotaConfig(),
		},
		want: want{
			enabled:   false,
			remaining: nil,
			mainSink: wantSink{
				bulks: repeat(99, 0),
				len:   0,
			},
			secondarySink: wantSink{
				bulks: repeat(99, 0),
				len:   0,
			},
		},
	}, {
		name: "cleanupping works",
		args: args{
			mainSink: emitterConfig(withCleanupping(17*time.Second, 28*time.Second)),
			secondarySink: emitterConfig(withDebouncerConfig(&logstore.DebouncerConfig{
				MinFrequency: 0,
				MaxBulkSize:  15,
			}), withCleanupping(5*time.Second, 47*time.Second)),
			quota: quotaConfig(),
		},
		want: want{
			enabled:   true,
			remaining: nil,
			mainSink: wantSink{
				bulks: repeat(1, 60),
				len:   21,
			},
			secondarySink: wantSink{
				bulks: repeat(15, 4),
				len:   18,
			},
		},
	}, {
		name: "when quota has a limit of 90, 30 are remaining",
		args: args{
			mainSink:      emitterConfig(),
			secondarySink: emitterConfig(),
			quota:         quotaConfig(withLimiting()),
		},
		want: want{
			enabled:   true,
			remaining: uint64Ptr(30),
			mainSink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
			secondarySink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
		},
	}, {
		name: "when quota has a limit of 30, 0 are remaining",
		args: args{
			mainSink:      emitterConfig(),
			secondarySink: emitterConfig(),
			quota:         quotaConfig(withLimiting(), withAmountAndInterval(30)),
		},
		want: want{
			enabled:   true,
			remaining: uint64Ptr(0),
			mainSink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
			secondarySink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
		},
	}, {
		name: "when quota has amount of 30 but is not limited, remaining is nil",
		args: args{
			mainSink:      emitterConfig(),
			secondarySink: emitterConfig(),
			quota:         quotaConfig(withAmountAndInterval(30)),
		},
		want: want{
			enabled:   true,
			remaining: nil,
			mainSink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
			secondarySink: wantSink{
				bulks: repeat(1, 60),
				len:   60,
			},
		},
	}}
	for _, ttt := range tests {
		t.Run("Given over a minute, each second a log record is emitted", func(tt *testing.T) {
			tt.Run(ttt.name, func(t *testing.T) {
				ctx := context.Background()
				clock := clock.NewMock()

				ttt.args.quota.PeriodStart = time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
				ttt.args.quota.PeriodEnd = ttt.args.quota.PeriodStart.Add(ttt.args.quota.Interval)
				clock.Set(ttt.args.quota.PeriodStart)

				mainStorage := emittermock.NewInMemoryStorage(clock)
				mainEmitter, err := logstore.NewEmitter(ctx, clock, ttt.args.mainSink, mainStorage)
				if err != nil {
					t.Errorf("expected no error but got %v", err)
					return
				}
				secondaryStorage := emittermock.NewInMemoryStorage(clock)
				secondaryEmitter, err := logstore.NewEmitter(ctx, clock, ttt.args.secondarySink, secondaryStorage)
				if err != nil {
					t.Fatalf("expected no error but got %v", err)
					return
				}

				svc := logstore.New(
					quotaqueriermock.NewNoopQuerier(&ttt.args.quota),
					logstore.UsageReporterFunc(func(context.Context, []*instance.QuotaNotifiedEvent) error { return nil }),
					mainEmitter,
					secondaryEmitter)

				if svc.Enabled() != ttt.want.enabled {
					t.Errorf("wantet service enabled to be %t but is %t", ttt.want.enabled, svc.Enabled())
					return
				}

				var (
					remaining *uint64
				)
				for i := 0; i < ticks; i++ {
					err = svc.Handle(ctx, emittermock.NewRecord(clock))
					runtime.Gosched()
					remaining, err = svc.Limit(ctx, "non-empty-instance-id")
					if err != nil {
						t.Fatalf("expected no error but got %v", err)
						return
					}
					clock.Add(tick)
				}
				time.Sleep(time.Millisecond)
				runtime.Gosched()

				mainBulks := mainStorage.Bulks()
				if !reflect.DeepEqual(ttt.want.mainSink.bulks, mainBulks) {
					t.Errorf("wanted main storage to have bulks %v, but got %v", ttt.want.mainSink.bulks, mainBulks)
				}

				mainLen := mainStorage.Len()
				if !reflect.DeepEqual(ttt.want.mainSink.len, mainLen) {
					t.Errorf("wanted main storage to have len %d, but got %d", ttt.want.mainSink.len, mainLen)
				}

				secondaryBulks := secondaryStorage.Bulks()
				if !reflect.DeepEqual(ttt.want.secondarySink.bulks, secondaryBulks) {
					t.Errorf("wanted secondary storage to have bulks %v, but got %v", ttt.want.secondarySink.bulks, secondaryBulks)
				}

				secondaryLen := secondaryStorage.Len()
				if !reflect.DeepEqual(ttt.want.secondarySink.len, secondaryLen) {
					t.Errorf("wanted secondary storage to have len %d, but got %d", ttt.want.secondarySink.len, secondaryLen)
				}

				if remaining == nil && ttt.want.remaining == nil {
					return
				}

				if remaining == nil && ttt.want.remaining != nil ||
					remaining != nil && ttt.want.remaining == nil {
					t.Errorf("wantet remaining nil %t but got %t", ttt.want.remaining == nil, remaining == nil)
					return
				}
				if *remaining != *ttt.want.remaining {
					t.Errorf("wantet remaining %d but got %d", *ttt.want.remaining, *remaining)
					return
				}
			})
		})
	}
}