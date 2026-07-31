# DB・API ローカル検証結果

- 検証日: 2026-07-31
- 対象: 現行 SQLite、AWS PostgreSQL 候補、S3/R2 オブジェクトアダプター
- 本番方針: AWS（ECS/Fargate、RDS for PostgreSQL、S3）
- Cloudflare: D1/R2/Containers は検証用途のみ

## 現行 SQLite の読み取り専用診断

`.data/zaiko.db` を `mode=ro` と `PRAGMA query_only = ON` で開き、変更を加えずに診断した。

| 項目 | 結果 |
| --- | ---: |
| integrity check | `ok` |
| 外部キー違反 | 0件 |
| 適用済み migration | 27件 |
| 最新 migration | `000027_purchase_request_groups` |
| ユーザーテーブル | 50 |
| 合計行数 | 493 |

## 移行アーティファクト検証

`cmd/dbexport` で全50テーブルを型付き NDJSON と manifest に出力し、続けて
`cmd/dbverify` で次を検証した。

- manifest と実ファイルの対応
- テーブル名・ファイル名・カラム名の安全性と重複
- 主キーカラム定義
- 行数
- SHA-256
- 各行のテーブル・カラム構造
- `NULL`、整数、実数、文字列、バイナリの型表現

検証は成功した。検証用アーティファクトはローカル一時領域から削除し、既存DBは変更していない。

## コード検証

以下はローカルで成功している。

- `go test ./...`
- `go vet ./...`
- Cloudflare Container router の TypeScript 型検査
- PostgreSQL候補DDLの静的安全性テスト
- PostgreSQLの商品登録・仕入・売上・出荷・返品在庫戻しWriterの契約・冪等性・競合テスト
- PostgreSQL接続設定の単体テスト
- AWS task role対応署名処理の単体テスト
- S3/R2/ローカルファイルのオブジェクトストア単体テスト
- オブジェクト二段階確定サービスの単体テスト
- D1検証用オブジェクトメタデータWriterのテナント境界・状態遷移・read-after-writeテスト
- D1 ServiceおよびCloudflare Container routerのTypeScript型検査
- SQLiteエクスポート・検証の改ざん検出テスト

## 未実施の外部環境検証

次はローカル環境だけでは完了できないため、本番切替条件に含める。

- PostgreSQL 16 へ migration を実適用するテスト
- RDS staging での制約・インデックス・クエリプラン確認
- RDS transaction、同時採番、CAS競合、ロールバック試験
- S3 staging への実アップロード、削除、Versioning、復旧試験
- RDS PITR、Multi-AZ failover、バックアップ復旧訓練
- ECS task role、Secrets Manager、SSM、ALB readiness の結合確認
- 匿名化データを使用した SQLite から PostgreSQL へのリハーサル
- 件数、金額、参照整合性、状態別件数の移行前後照合
- 現行HTTPハンドラーをPostgreSQL/S3 providerへ切り替えた結合・回帰試験

本番データ、本番RDS、本番S3へは接続・書込みを行っていない。

現時点の通常画面は既存SQLite経路を維持している。PostgreSQL/S3/D1/R2実装は
切替前の候補アダプターおよび検証用経路であり、外部staging検証と業務仕様確定前に
本番の正本へ切り替えない。

## 未確定の業務仕様

次の項目はDB/API実装で独自に固定せず、仕様決定後に契約とmigrationを追加する。

- 税率、免税、端数処理の確定ルール
- 外貨換算の基準時点、レート取得元、丸め
- 一つの仕入明細数量から複数商品を生成する場合の属性入力
- 売上更新時の `ExpectedVersion` が指す集約
- 部分出荷時の数量・分割・再出荷
- 返品後コンディションの正規化と変更履歴
- 複数返品を一括確定した場合のバージョン応答
- 承認OTPの最終運用

これらが未確定でも、現行UI・SQLite経路を維持したままPostgreSQL/S3アダプターの段階導入と検証は継続できる。
