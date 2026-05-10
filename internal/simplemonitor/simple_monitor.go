package simplemonitor

import (
	"context"
	"errors"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
)

// ErrSimpleMonitorNotFound indicates that the SakuraCloud simple monitor no longer exists.
var ErrSimpleMonitorNotFound = errors.New("sakura simple monitor not found")

// SimpleMonitorClient は SakuraCloud シンプル監視を同期・確認するための最小 API を表す。
type SimpleMonitorClient interface {
	// Create は Kubernetes リソースに対応する SakuraCloud シンプル監視を新規作成し、作成された monitor ID を返す。
	// status.monitorID が未設定の場合、または保存済み monitor ID の更新で not found を検出した場合だけ呼び出す。
	Create(ctx context.Context, desired SimpleMonitorDesired) (string, error)

	// CheckSynced は SakuraCloud シンプル監視を 1 回だけ読み取り、desired state と一致していることを確認する。
	// 同期済み generation の 24 時間ごとの確認にだけ使い、差分・削除・読み取り失敗は error として返す。
	CheckSynced(ctx context.Context, id string, desired SimpleMonitorDesired) error

	// Update は保存済み monitor ID の SakuraCloud シンプル監視を desired state に更新する。
	// spec が変わった generation でだけ呼び出し、Sakura 側で削除済みの場合は ErrSimpleMonitorNotFound を返す。
	Update(ctx context.Context, id string, desired SimpleMonitorDesired) error

	// Delete は保存済み monitor ID の SakuraCloud シンプル監視を削除する。
	// Sakura 側で削除済みの場合は ErrSimpleMonitorNotFound を返す。
	Delete(ctx context.Context, id string) error
}

// SimpleMonitorDesired is the SakuraCloud simple monitor shape derived from the Kubernetes resource.
type SimpleMonitorDesired struct {
	Target         string
	Description    string
	Tags           []string
	Protocol       monitoringv1alpha1.HealthCheckProtocol
	Port           int32
	Path           string
	ExpectedStatus int32
	TimeoutSeconds int32
	HTTP2          bool
	Interval       int32
	RetryInterval  int32
	WebhookURL     string
	RepeatInterval int32
}
