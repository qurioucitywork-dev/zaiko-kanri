# Cloudflare 検証から AWS 本番への引継ぎ

## 1. 確定事項

- Cloudflare D1 / R2 / Containers は適合性テスト専用。
- 最終本番は AWS ECS / RDS for PostgreSQL / S3。
- Cloudflare の検証データを本番へ昇格しない。
- D1 と PostgreSQL の dual-write、常時同期、障害時の SQLite fallback は行わない。
- 現行 SQLite は本番移行の reconciliation と rollback 判定が完了するまで暗号化して保持する。

## 2. 現在実装した境界

- `internal/dataaccess`: provider-neutral read contracts と DTO。
- `internal/dataaccess/sqliteadapter`: 現行 SQLite に対する read adapter。
- `internal/dataaccess/d1adapter`: D1 内部 Worker API に対する read adapter。
- `internal/dataaccess/contracttest`: tenant、NotFound、金額、日付、sort、pagination、診断情報の契約。
- `deploy/cloudflare/container-router`: Container ルーティングと outbound handler。
- `deploy/cloudflare/d1-service`: 外部非公開、read-only、業務 API 限定の D1 Worker。
- `deploy/aws`: ECS/RDS/S3 本番引継ぎ条件。

これらは段階移行の土台であり、現行 `internal/database.Store` を置き換えてはいない。現在の UI、
既存 API、SQLite 書込み処理は従来どおり動作する。

## 3. Cloudflare 検証ゲート

1. baseline candidate が migration `000001`〜`000027` の最終形と一致する。
2. D1 capability test:
   - foreign key
   - UNIQUE / CHECK
   - partial index
   - trigger
   - batch と競合
   - 金額最大値と文字列化
   - export / restore
3. D1 adapter contract test。
4. 合成データだけを使った read reconciliation。
5. 書込み repository は一業務 transaction を一回の内部 API request で完結させる。
6. R2 二段階処理と checksum / orphan / missing recovery test。
7. P0 / P1 指摘が 0。

## 4. AWS 本番ゲート

1. PostgreSQL adapter が SQLite / D1 と同じ contract を満たす。
2. PostgreSQL 用 migration を SQLite / D1 と同一の論理版 manifest で追跡する。
3. staging RDS で load、競合、rollback、PITR、restore を実地確認する。
4. staging S3 で Versioning、restore、checksum、orphan / missing reconciliation を確認する。
5. migration job -> ECS rollout の順序を自動化する。
6. read-only window、DNS/ALB 切替、rollback 条件、責任者、RTO/RPO を確定する。
7. 監視と alert を有効化し、復旧リハーサルを完了する。

## 5. 未実装のため切替を禁止する項目

- D1 write repositories と業務 transaction API。
- R2/S3 object adapter と二段階アップロード。
- PostgreSQL adapter と PostgreSQL migration。
- session、認証、承認、採番、在庫、売上、出荷、返品、棚卸の provider-neutral write contract。
- SQLite -> PostgreSQL 一方向 migration tool。
- RDS/S3 staging 復旧試験。

上記が完了するまでは Cloudflare URL 切替、本番データ投入、AWS 本番切替を行わない。

## 6. 安全確認

ローカルで次を実行する。

```powershell
.\scripts\db-api-safety-check.ps1 -RunGoTests
```

このスクリプトは既存 migration hash、Cloudflare binding 分離、外部公開禁止、任意 SQL surface、
Go 回帰テストを検査する。Cloudflare や AWS のリソースは作成・変更しない。
