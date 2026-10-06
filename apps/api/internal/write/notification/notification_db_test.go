//go:build dbtest

package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 通知の設定と送った印の操作を、本物の MySQL で確かめる。LINE の連携の操作は、
// 読み取りと一緒に internal/feature/line の line_db_test.go（sqlLineStore）が確かめる。
func TestNotificationDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	prefs := func(fx dbtest.Fixture, user string) []string {
		return fx.Rows(`SELECT morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled
			FROM NotificationPreference WHERE userId = ?`, user)
	}

	t.Run("LINE と連携していなければ LINE 通知は ON にできない。メールだけなら保存できる", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		if err := SavePreference(ctx, db, user, Preference{LineMorningEnabled: true}, now); !errors.Is(err, ErrLineNotConnected) {
			t.Fatalf("err = %v", err)
		}
		if got := prefs(fx, user); len(got) != 0 {
			t.Errorf("断ったのに行ができた: %v", got)
		}
		if err := SavePreference(ctx, db, user, Preference{EmailMorningEnabled: true}, now); err != nil {
			t.Fatal(err)
		}
		want := `{"eveningEnabled":"0","lineEveningEnabled":"0","lineMorningEnabled":"0","morningEnabled":"1"}`
		if got := prefs(fx, user); len(got) != 1 || got[0] != want {
			t.Errorf("保存した行: %v", got)
		}
	})

	t.Run("連携していれば LINE 通知を ON にでき、2回目は同じ行を書き換える。解除で LINE だけ落ちる", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		fx.LineConnection(user)
		if err := SavePreference(ctx, db, user, Preference{EmailEveningEnabled: true, LineMorningEnabled: true}, now); err != nil {
			t.Fatal(err)
		}
		if err := SavePreference(ctx, db, user, Preference{EmailEveningEnabled: true, LineEveningEnabled: true}, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		want := `{"eveningEnabled":"1","lineEveningEnabled":"1","lineMorningEnabled":"0","morningEnabled":"0"}`
		if got := prefs(fx, user); len(got) != 1 || got[0] != want {
			t.Errorf("書き換えた行: %v", got)
		}

		if err := Disconnect(ctx, db, user, now); err != nil {
			t.Fatal(err)
		}
		want = `{"eveningEnabled":"1","lineEveningEnabled":"0","lineMorningEnabled":"0","morningEnabled":"0"}`
		if got := prefs(fx, user); len(got) != 1 || got[0] != want {
			t.Errorf("解除のあと: %v", got)
		}
		if err := SavePreference(ctx, db, user, Preference{LineEveningEnabled: true}, now); !errors.Is(err, ErrLineNotConnected) {
			t.Errorf("解除のあとに LINE 通知を ON にできた: %v", err)
		}
	})

	t.Run("送った印は同じ日・時間帯・経路に1つだけ。消せばまた入る", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		d := Delivery{UserID: user, Date: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), Slot: "morning", Channel: "line"}
		id, dup, err := MarkDelivery(ctx, db, d, now)
		if err != nil || dup || id == 0 {
			t.Fatal(id, dup, err)
		}
		if _, dup, err := MarkDelivery(ctx, db, d, now); err != nil || !dup {
			t.Fatalf("2回目が重複にならない: %v %v", dup, err)
		}
		other := d
		other.Channel = "email"
		if _, dup, err := MarkDelivery(ctx, db, other, now); err != nil || dup {
			t.Fatalf("経路が違うのに重複になった: %v %v", dup, err)
		}
		if err := UnmarkDelivery(ctx, db, id); err != nil {
			t.Fatal(err)
		}
		if _, dup, err := MarkDelivery(ctx, db, d, now); err != nil || dup {
			t.Fatalf("消したあとに入らない: %v %v", dup, err)
		}
	})
}
