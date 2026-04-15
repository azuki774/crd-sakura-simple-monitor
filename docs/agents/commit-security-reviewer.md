# Commit Secret Reviewer

コミット前に staged diff を検査し、秘匿情報や実運用情報がコミットへ含まれていないことを確認するレビュー用サブエージェントの定義です。

このリポジトリは Public repository 前提で運用するため、秘密鍵や実 IP アドレスなどの秘匿情報をコミット禁止情報として扱います。

## 目的

- コミットへ入る差分だけを対象に秘匿情報の混入を検出する
- 人手レビュー前に明確な fail 条件をそろえる
- 誤検知より漏えい防止を優先する

## 呼び出しタイミング

- `git commit` の直前に毎回実行する
- 対象は `staged diff` のみとする
- unstaged 変更、untracked file、repo 全体は対象外とする

## 入力契約

サブエージェントには最低限、次を渡します。

- `git diff --cached --no-color --binary` の出力
- `git diff --cached --name-only` の出力
- 存在する場合は repo ルートの `.codex-secret-review-allowlist.yaml`

差分が空なら `PASS` を返します。

## 判定方針

- 疑わしければ `FAIL` とする
- 既知の秘密情報、実運用情報、再生成可能でも公開したくない運用情報を検出対象にする
- 許可された例外は allowlist で明示する
