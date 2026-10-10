// Package hostfault は、ホストの層の障害（カオス段階2、JUK-174）を、本番の EC2 に SSM Run Command で起こす。
// アプリの層の障害（internal/fault）と同じ表（ChaosExperiment）に記録し、予告なしのくじ（feature/chaos/schedule.go）だけが起こす。
//
// 起こす中身は SSM ドキュメント juken-map-chaos-host（terraform/chaos.tf、スクリプトは terraform/chaos/host-fault.sh）。
// どの障害も EC2 の systemd の一時的なユニットとして動き、終わる時刻が来たら systemd が止めて元に戻す。API のプロセスや
// SSM Agent が落ちても戻る。EC2 のロールが送れるのはこのドキュメントを自分のインスタンスへだけ（terraform/iam_ec2.tf）。
package hostfault

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
)

// 種類。ChaosExperiment.kind には "host_" を付けて入れ、SSM ドキュメントには付けずに渡す。
const (
	KindProcessKill fault.Kind = "host_process_kill" // API のプロセスを kill -9 する（コンテナの再起動の方針で戻るか）
	KindCPU         fault.Kind = "host_cpu"          // stress-ng で CPU を使い切る
	KindMemory      fault.Kind = "host_memory"       // stress-ng でメモリを使う
	KindDisk        fault.Kind = "host_disk"         // ルートのディスクを埋める
	KindDBDelay     fault.Kind = "host_db_delay"     // tc netem で RDS への通信を遅らせる
	KindDBLoss      fault.Kind = "host_db_loss"      // tc netem で RDS への通信を落とす
)

// Kinds はホストの層で起こせる種類の全部。
var Kinds = []fault.Kind{KindProcessKill, KindCPU, KindMemory, KindDisk, KindDBDelay, KindDBLoss}

// Levels は種類ごとに起こせる強さ。cpu・memory・db_loss は %、disk は埋めたあとの使用率の %、db_delay は ms。
// SSM ドキュメントのスクリプトも同じ値だけを受け付ける（hostfault_test.go が突き合わせる）。
var Levels = map[fault.Kind][]int{
	KindProcessKill: {0},
	KindCPU:         {50, 80, 100},
	KindMemory:      {50, 70, 90},
	KindDisk:        {90, 95, 98},
	KindDBDelay:     {100, 300, 1000},
	KindDBLoss:      {10, 30, 100},
}

// ProcessKillDuration は kill の実験の長さ。kill するのは始めの1回だけで、この間に戻るかを見る。
const ProcessKillDuration = 10 * time.Minute

// IsHost は kind がホストの層の種類か。
func IsHost(kind fault.Kind) bool {
	_, ok := Levels[kind]
	return ok
}

// Spec は kind を level の強さで d 続ける実験の中身（ChaosExperiment の行の値）。route は使わないので空にする。
func Spec(kind fault.Kind, level int, d time.Duration) fault.Spec {
	s := fault.Spec{Kind: kind, Duration: d, Rate: 1}
	switch kind {
	case KindDBDelay:
		s.DelayMs = level
	case KindCPU, KindMemory, KindDisk, KindDBLoss:
		s.Rate = float64(level) / 100
	}
	return s
}

// Level は記録した実験の強さ（Spec の逆）。
func Level(e fault.Experiment) int {
	switch e.Kind {
	case KindDBDelay:
		return e.DelayMs
	case KindCPU, KindMemory, KindDisk, KindDBLoss:
		return int(math.Round(e.Rate * 100))
	}
	return 0
}

// Runner はホストで障害を起こし・戻す。本番は SSM、テストは偽物。
type Runner interface {
	// Start は e を起こし、ホストで始まったら戻る。始められなければ誤り。
	Start(ctx context.Context, e fault.Experiment) error
	// Revert は起こしている障害を全部戻す。何も起こしていなければ何もしない。
	Revert(ctx context.Context) error
}

// DocumentName は SSM ドキュメントの名前（terraform/chaos.tf）。
const DocumentName = "juken-map-chaos-host"

// commandWait は送ったコマンドが終わるのを待つ上限。初回は stress-ng を apt で入れるので長めにする。
const commandWait = 5 * time.Minute

// SSM は SSM Run Command で、この API が動いている EC2 に障害を起こす。
type SSM struct {
	client     *ssm.Client
	instanceID string
}

// NewSSM は EC2 のメタデータ（IMDS）から自分のインスタンスとリージョンを調べ、EC2 のロールで SSM を呼ぶ準備をする。
func NewSSM(ctx context.Context) (*SSM, error) {
	doc, err := imds.New(imds.Options{}).GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return nil, fmt.Errorf("instance identity: %w", err)
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(doc.Region))
	if err != nil {
		return nil, err
	}
	return &SSM{client: ssm.NewFromConfig(cfg), instanceID: doc.InstanceID}, nil
}

func (s *SSM) Start(ctx context.Context, e fault.Experiment) error {
	if !IsHost(e.Kind) {
		return fmt.Errorf("not a host kind: %s", e.Kind)
	}
	return s.send(ctx, map[string][]string{
		"Action":  {"start"},
		"Kind":    {strings.TrimPrefix(string(e.Kind), "host_")},
		"Seconds": {strconv.Itoa(int(e.EndsAt.Sub(e.StartsAt).Seconds()))},
		"Level":   {strconv.Itoa(Level(e))},
	})
}

func (s *SSM) Revert(ctx context.Context) error {
	return s.send(ctx, map[string][]string{"Action": {"revert"}})
}

// send はコマンドを送り、終わるまで待つ。失敗（スクリプトが 0 以外で終わった・時間切れ）なら誤り。
func (s *SSM) send(ctx context.Context, params map[string][]string) error {
	out, err := s.client.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: aws.String(DocumentName),
		InstanceIds:  []string{s.instanceID},
		Parameters:   params,
	})
	if err != nil {
		return err
	}
	id := out.Command.CommandId
	err = ssm.NewCommandExecutedWaiter(s.client).Wait(ctx,
		&ssm.GetCommandInvocationInput{CommandId: id, InstanceId: aws.String(s.instanceID)}, commandWait)
	if err != nil {
		return errors.Join(fmt.Errorf("command %s", aws.ToString(id)), err)
	}
	return nil
}
