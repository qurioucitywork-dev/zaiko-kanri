# Cloudflare 検証環境

このディレクトリは Cloudflare D1 / R2 / Containers の適合性を検証するための構成例である。
本番環境では使用しない。最終本番は AWS ECS / RDS for PostgreSQL / S3 とする。

## 安全境界

- `wrangler.example.jsonc` の ID はプレースホルダーであり、リポジトリへ実 ID や secret を保存しない。
- D1 binding は `d1-service` だけが保持する。
- `d1-service` は route、custom domain、`workers.dev` を持たず、Service Binding からだけ呼び出す。
- Container Router は D1 binding を持たない。
- Container は `http://d1.internal` へ業務 API を呼び、outbound handler が Service Binding へ転送する。
- 任意 SQL、任意テーブル、任意 WHERE を受け取るエンドポイントは禁止する。
- D1 障害時に SQLite へ自動フォールバックしない。dual-write もしない。
- 検証データは合成または不可逆匿名化データだけを使用する。
- `wrangler deploy` と remote migration は、利用者の明示承認なしに実行しない。

## 構成

```text
Browser
  -> container-router Worker
  -> Cloudflare Container (Go)
  -> http://d1.internal
  -> container outbound handler
  -> D1_SERVICE Service Binding
  -> d1-service Worker
  -> D1
```

R2 はこの最小構成にはまだ接続しない。商品画像移行では、`pending -> ready` の二段階処理、
checksum 検証、補償処理、孤児検出が contract test を通った後にテスト専用 bucket を接続する。

## ローカル検証順序

1. `docs/db-api/baseline-candidate-000027.sql` を新しいローカル D1 だけへ適用する。
2. 合成 seed を適用する。本番 SQLite の export をそのまま使用しない。
3. `d1-service` の read contract と capability test を実行する。
4. `container-router` から `d1.internal` 経由の接続を確認する。
5. 件数、金額、参照、状態を read-only reconciliation で照合する。
6. 失敗時は D1 検証 DB を破棄し、現行 SQLite へ変更を加えない。

現在の Go composition root は SQLite の `Store` に直接依存しているため、この構成をデプロイ可能と
みなしてはならない。D1 adapter を各業務 repository へ段階的に接続し、全 contract test が成功して
から Container 用起動コマンドを有効化する。

## 参照

- <https://developers.cloudflare.com/containers/platform-details/workers-connections/>
- <https://developers.cloudflare.com/containers/platform-details/outbound-traffic/>
- <https://developers.cloudflare.com/d1/>
- <https://developers.cloudflare.com/r2/api/s3/api/>
