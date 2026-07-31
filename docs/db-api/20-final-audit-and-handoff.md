# DB・API 最終監査・AWS引継ぎ

- 監査日: 2026-07-31
- 最終本番方針: AWS（ECS/Fargate、RDS for PostgreSQL、S3）
- Cloudflare: D1/R2/Containersはテスト候補のみ
- 現行画面: SQLite経路を維持

## 1. 最終判定

| 区分 | 件数 |
| --- | ---: |
| P0 | 0 |
| P1 | 0 |
| ローカル実装P2 | 0 |

PostgreSQL、S3/R2、D1の候補実装は、ローカル実装候補として監査合格とする。
ただし、外部staging検証が未完了のため、本番切替可能とは判定しない。

## 2. 監査済みの重要要件

- PostgreSQLの組織・ユーザー・権限検証
- D1の停止組織拒否、権限検証、テナント分離
- テナント所有データの複合外部キー
- 商品、仕入、売上、出荷、返品在庫戻しのトランザクション境界
- PostgreSQL全workflowの冪等性
- D1 Go AdapterとWorker間のcanonical hash一致
- 再送時刻のみが変わる再試行の安全な再生
- operation、tenant、actor、business payload変更時の不一致検出
- `&`、`<`、`>`、日本語、U+2028/U+2029を含むJSON正規化
- オブジェクト保存の再実行、応答喪失、補償、孤児化防止
- PostgreSQL検証SQLのfail-fast動作
- 既存SQLiteマイグレーションの不変性

## 3. ローカル検証結果

- `go test ./...`: 成功
- 重要adapter/serviceテスト10回反復: 成功
- `go vet ./...`: 成功
- `go mod verify`: 成功
- D1 Worker TypeScript型検査: 成功
- Cloudflare Container Router TypeScript型検査: 成功
- `gofmt`差分: なし
- `git diff --check`: エラーなし（Windows改行警告のみ）
- DB/API safety check: 成功
- リモート資源変更: なし

Race detectorは現在のWindows用GoがCGO無効のため未実行であり、外部CIまたはLinux環境で実施する。

## 4. 本番切替前の必須外部検証

1. PostgreSQL 16へmigrationと`verify_schema.sql`を実適用する。
2. 隔離RDS stagingで制約、索引、クエリプラン、transaction、同時採番、CAS、rollbackを確認する。
3. 隔離S3 stagingでupload、HEAD、checksum、delete、Versioning、復旧、応答喪失、孤児reconciliationを確認する。
4. RDS PITR、Multi-AZ failover、バックアップ復旧訓練を実施する。
5. ECS task role、Secrets Manager、SSM、ALB readinessを結合確認する。
6. 匿名化SQLiteデータでPostgreSQL移行リハーサルを実施し、件数、金額、外部キー、ステータスを前後照合する。
7. 現行HTTP handlerをPostgreSQL/S3 providerへ切り替えた環境でE2E・回帰試験を行う。
8. Cloudflare候補を残す場合は、D1/R2 Service BindingをMiniflareまたはremote test環境で結合確認する。

これらが成功するまで、現行SQLiteを正本とし、PostgreSQL/S3へ本番切替しない。

## 5. 業務仕様の未確定事項

- 税、免税、外貨換算、端数処理の確定ルール
- 1仕入明細の数量から複数商品を生成する場合の属性入力
- 売上更新時の`ExpectedVersion`が指す集約
- 部分出荷、分納、再出荷の数量ルール
- 返品後コンディションの正規化と変更履歴
- 複数返品を一括確定した場合のversion応答
- 承認OTPの最終運用仕様

未確定事項は現行画面や既存SQLite処理へ独自判断で混在させず、仕様確定後に追加migrationと契約テストで導入する。

## 6. 移行時の注意

- 既存SQLite migration `000001`から`000027`を変更しない。
- 本番DBや本番S3へ直接テスト投入しない。
- 先にバックアップと復旧手順を検証する。
- dual writeや暗黙fallbackを導入しない。
- 切替はreadiness確認、移行、前後照合、provider切替の順で行う。
- 切替失敗時は書込みを停止し、旧正本を再利用できる状態で復旧する。

## 7. UIへの影響

今回追加したPostgreSQL/S3/D1/R2実装は候補adapterと検証経路であり、現行HTTP handlerのDB providerは切り替えていない。
そのため通常画面は引き続きSQLiteを使用する。

作業開始時からworktreeには多数の未コミットUI変更があるため、Git履歴だけでDB/API作業単独のUI非変更を厳密に帰属することはできない。
provider切替時にはモック一致済みUIを基準に、全画面のE2E・回帰試験を必須とする。
