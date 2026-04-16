# crd-sakura-simple-monitor

Kubernetes CRD / Controller で、さくらのクラウドのシンプル監視を宣言的に管理するためのプロジェクトです。

現時点では、Kubebuilder ベースの operator scaffold と `SakuraSimpleMonitor` API の初期型定義を含みます。Go 型を正本とし、CRD の `openAPIV3Schema` は `controller-gen` で生成する前提です。

## 主なディレクトリ

- `api/v1alpha1/`: CRD の Go 型定義
- `internal/controller/`: controller-runtime ベースの controller 雛形
- `config/`: CRD / RBAC / manager / sample manifest
- `docs/`: 補足ドキュメント

## 開発コマンド

`nix develop` で開発シェルに入ると、静的リンク向けの `go` と `kubebuilder`, `kubectl` が利用できます。

- `make manifests`: CRD と RBAC を生成
- `make generate`: DeepCopy などのコード生成
- `make test`: envtest を使った unit test
- `make build`: manager バイナリをビルド
- `make run`: ローカルで controller を起動

## 現在の API 方針

- `apiVersion`: `monitoring.k8s.azuki.blue/v1alpha1`
- `kind`: `SakuraSimpleMonitor`
- `spec` は `target`, `healthCheck`, `interval`, `retryInterval`, `notifications`, `description`
- `status` は `monitorID`, `observedGeneration`, `conditions`, `health`, `lastSyncedAt`
