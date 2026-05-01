# crd-sakura-simple-monitor

Kubernetes CRD / Controller で、さくらのクラウドのシンプル監視を宣言的に管理するためのプロジェクトです。

現時点では、Kubebuilder ベースの no-op operator と `SakuraSimpleMonitor` API の初期型定義を含みます。Go 型を正本とし、CRD の `openAPIV3Schema` は `controller-gen` で生成する前提です。

manager プロセスは起動し、`SakuraSimpleMonitor` リソースを watch します。現在の reconcile は対象リソースを取得してログを出力するだけで、さくらのクラウド API 呼び出し、外部リソース作成、`status` 更新、finalizer 処理はまだ行いません。

## デプロイ方針

現段階の operator は no-op としてデプロイできます。CRD、RBAC、manager Deployment を適用すると manager は起動し、`SakuraSimpleMonitor` リソースの watch とログ出力だけを行います。さくらのクラウド API 認証情報や通知先 Secret は不要です。

他リポジトリの manifest からは、GitHub Container Registry の image を参照します。

- `ghcr.io/azuki774/crd-sakura-simple-monitor:latest`: `master` の最新 image
- `ghcr.io/azuki774/crd-sakura-simple-monitor:<commit-sha>`: 特定 commit の image

再現性が必要な環境では commit SHA tag を使い、動作確認や暫定デプロイでは `latest` を使います。

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
