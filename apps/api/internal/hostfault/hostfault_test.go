package hostfault

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
)

func TestSpecAndLevel(t *testing.T) {
	for _, kind := range Kinds {
		for _, level := range Levels[kind] {
			s := Spec(kind, level, 10*time.Minute)
			if s.Rate <= 0 || s.Rate > 1 {
				t.Errorf("Spec(%s, %d).Rate = %v", kind, level, s.Rate)
			}
			if got := Level(fault.Experiment{Kind: kind, Rate: s.Rate, DelayMs: s.DelayMs}); got != level {
				t.Errorf("Level(Spec(%s, %d)) = %d", kind, level, got)
			}
		}
	}
	if IsHost(fault.KindLatency) || !IsHost(KindDisk) {
		t.Error("IsHost")
	}
}

// SSM ドキュメントのスクリプトと terraform が受け付ける種類・強さが、Levels と同じか。
func TestScriptAcceptsLevels(t *testing.T) {
	script := read(t, "../../../../terraform/chaos/host-fault.sh")
	tf := read(t, "../../../../terraform/chaos.tf")
	for _, kind := range Kinds {
		name := strings.TrimPrefix(string(kind), "host_")
		want := make([]string, len(Levels[kind]))
		for i, l := range Levels[kind] {
			want[i] = strconv.Itoa(l)
		}
		line := regexp.MustCompile(`(?m)^\s*` + name + `\) allowed="([^"]*)"`).FindStringSubmatch(script)
		if line == nil || line[1] != strings.Join(want, " ") {
			t.Errorf("host-fault.sh の %s: %v, want %q", name, line, strings.Join(want, " "))
		}
		if !strings.Contains(tf, `"`+name+`"`) {
			t.Errorf("chaos.tf の Kind に %s が無い", name)
		}
	}
	// SSM は {{ 名前 }} をパラメーターに置き換えるので、決めたもの以外を書かない。
	for _, m := range regexp.MustCompile(`\{\{[^}]*\}\}`).FindAllString(script, -1) {
		switch m {
		case "{{ Action }}", "{{ Kind }}", "{{ Seconds }}", "{{ Level }}", "{{ DatabaseHost }}":
		default:
			t.Errorf("host-fault.sh に %s がある", m)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
