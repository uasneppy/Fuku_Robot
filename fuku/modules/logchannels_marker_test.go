//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
)

// TestSetLogMarkerSurvivesFailedSave holds that a /setlog forward whose save fails gives the
// marker back and says so: the administrator's next forward of the same post binds the channel.
func TestSetLogMarkerSurvivesFailedSave(t *testing.T) {
	for _, store := range []string{"memory", "miniredis"} {
		t.Run(store, func(t *testing.T) {
			withRealLocales(t)
			if store == "miniredis" {
				withMiniredis(t)
			}
			env := newSetLogFlowEnv(t)
			post := channelPostUpdate(env.channel, "/setlog")
			env.process(post)

			var failNext atomic.Int32
			failNext.Store(1)
			fault := "test:fail_log_channels_create_" + t.Name()
			err := db.DB.Callback().Create().Before("gorm:create").Register(fault, func(tx *gorm.DB) {
				if tx.Statement.Table == "log_channels" && failNext.Load() > 0 {
					failNext.Add(-1)
					_ = tx.AddError(errors.New("database is down"))
				}
			})
			if err != nil {
				t.Fatalf("register callback: %v", err)
			}
			t.Cleanup(func() {
				if err := db.DB.Callback().Create().Remove(fault); err != nil {
					t.Errorf("remove callback: %v", err)
				}
			})

			env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
			if left := failNext.Load(); left != 0 {
				t.Fatal("the save was never attempted, so the test did not reach its scenario")
			}
			if got := env.boundChannel(); got != 0 {
				t.Fatalf("a failed save bound %d", got)
			}
			if got, want := flowLastText(env.client, env.group.Id), ukText(t, "common_settings_save_failed"); got != want {
				t.Fatalf("reply after the failed save = %q, want %q", got, want)
			}

			env.process(forwardedPostUpdate(env.group, env.admin, post, post.ChannelPost.MessageId))
			if got := env.boundChannel(); got != env.channel.Id {
				t.Fatalf("the forward after the failed save bound %d, want %d", got, env.channel.Id)
			}
		})
	}
}

// TestConsumePendingMarkerOnce holds that without Redis concurrent consumers of one marker take
// it exactly once.
func TestConsumePendingMarkerOnce(t *testing.T) {
	const keys, workers = 100, 8
	for k := range keys {
		key := fmt.Sprintf("fuku:setlog:test:%d:%d", uniqueModuleChatID(), k)
		if err := storePendingMarker(key, []byte("1")); err != nil {
			t.Fatalf("storePendingMarker: %v", err)
		}
		var won atomic.Int32
		var start, done sync.WaitGroup
		start.Add(1)
		for range workers {
			done.Add(1)
			go func() {
				defer done.Done()
				start.Wait()
				if consumePendingMarker(key) {
					won.Add(1)
				}
			}()
		}
		start.Done()
		done.Wait()
		if got := won.Load(); got != 1 {
			t.Fatalf("marker %d was consumed %d times, want once", k, got)
		}
	}
}
