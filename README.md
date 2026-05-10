# crd-sakura-simple-monitor

Kubernetes CRD / Controller で、さくらのクラウドのシンプル監視を宣言的に管理するためのプロジェクトです。

現時点では、Kubebuilder ベースの operator と `SakuraSimpleMonitor` API の初期同期実装を含みます。Go 型を正本とし、CRD の `openAPIV3Schema` は `controller-gen` で生成する前提です。

manager プロセスは `SakuraSimpleMonitor` リソースを watch し、spec に対応するさくらのクラウドのシンプル監視を作成・更新します。同期済みリソースは 24 時間に 1 回だけさくらのクラウド API から設定を読み取り、spec と一致していることを確認します。削除時は finalizer により、status に保存した monitor ID の外部リソースを削除してから Kubernetes リソースの削除を完了します。既存シンプル監視の採用はまだ行いません。

## 利用方法

利用先の Kubernetes cluster に接続した状態で、さくらのクラウド API 認証情報を Secret として作成します。

```sh
kubectl create namespace crd-sakura-simple-monitor-system
kubectl create secret generic sakuracloud-api \
  -n crd-sakura-simple-monitor-system \
  --from-literal=access-token="${SAKURACLOUD_ACCESS_TOKEN}" \
  --from-literal=access-token-secret="${SAKURACLOUD_ACCESS_TOKEN_SECRET}"
```

controller をデプロイします。通常は GitHub Container Registry の image を指定します。

```sh
make deploy IMG=ghcr.io/azuki774/crd-sakura-simple-monitor:latest
```

監視対象は `SakuraSimpleMonitor` リソースとして作成します。

```sh
kubectl apply -f examples/sakurasimplemonitor.yaml
kubectl get sakurasimplemonitors -A
```

`spec.target` に監視対象の host または IP address を指定し、`spec.healthCheck` に protocol、port、path、期待する HTTP status code、timeout を設定します。通知は `spec.notifications.webhookURL`、`message`、`repeatInterval` で指定します。

`SakuraSimpleMonitor` を削除すると、controller は status に保存した monitor ID を使ってさくらのクラウド側のシンプル監視も削除します。DELETE に失敗した場合は API 連打を避けるため、finalizer を残したまま 4 時間後に再試行します。

## デプロイ方針

CRD、RBAC、manager Deployment を適用すると manager は起動し、`SakuraSimpleMonitor` リソースをさくらのクラウド API に同期します。manager Deployment は `sakuracloud-api` Secret から API 認証情報を読みます。

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sakuracloud-api
  namespace: system
type: Opaque
stringData:
  access-token: "<SAKURACLOUD_ACCESS_TOKEN>"
  access-token-secret: "<SAKURACLOUD_ACCESS_TOKEN_SECRET>"
```

作成・更新されるシンプル監視にはタグを付与しません。さくらのクラウド API のタグ形式制約で同期が失敗することを避けるため、Kubernetes リソースとの紐付けは `SakuraSimpleMonitor` の status に保存する monitor ID で行います。

他リポジトリの manifest からは、GitHub Container Registry の image を参照します。

- `ghcr.io/azuki774/crd-sakura-simple-monitor:latest`: `master` の最新 image
- `ghcr.io/azuki774/crd-sakura-simple-monitor:<commit-sha>`: 特定 commit の image

再現性が必要な環境では commit SHA tag を使い、動作確認や暫定デプロイでは `latest` を使います。

## 主なディレクトリ

- `api/v1alpha1/`: CRD の Go 型定義
- `internal/controller/`: controller-runtime ベースの controller 雛形
- `config/`: CRD / RBAC / manager / sample manifest
- `examples/`: `SakuraSimpleMonitor` リソースの作成例
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
- `status` は `monitorID`, `observedGeneration`, `conditions`, `lastSyncedAt`
