# AWS PostgreSQL本番物理設計候補

- ステータス: **Candidate / 本番未適用**
- 対象: AWS ECS + RDS for PostgreSQL + S3 の最終本番
- 対象DB: PostgreSQL 16以上
- 作成日: 2026-07-31

## 1. 結論

`deploy/aws/postgres/` に、新規RDSデータベースへ適用するPostgreSQL初期物理スキーマ候補を作成した。
この候補は、現行SQLite migrationをPostgreSQLへ逐語変換したものではない。モック論理モデル、
現行SQLite最終スキーマ、`internal/dataaccess` の共通契約を統合した、AWS本番向けの
**fresh database baseline** である。

次の操作は行っていない。

- 現行SQLite migrationまたはSQLite DBの変更
- UI、Handler、既存APIの変更
- AWS、RDS、S3、Cloudflare等のremote resourceへの接続・適用
- 本番データの変換・投入
- PostgreSQL adapterの実装

候補はstaging RDSでの構文検証、contract test、データ移行リハーサル、負荷試験、PITR試験に合格するまで
本番へ適用してはならない。

## 2. 参照した正本

| 正本 | 本設計で採用した内容 |
|---|---|
| `docs/db-api/01-mock-logical-model.md` | 在庫、仕入、売上、出荷、返品、BOX、ゲスト、購入依頼、承認、棚卸、相場の集約 |
| `docs/db-api/06-mrc-dto-mapping.md` | 内部IDと業務番号の分離、状態code、日付、日時、金額、為替、snapshotの表現 |
| `docs/db-api/09-cloudflare-aws-handoff.md` | AWSを最終本番とし、D1/SQLiteとのdual-writeやfallbackを行わない方針 |
| `docs/db-api/10-write-and-object-contract.md` | tenant、冪等性、CAS、業務transaction、監査、S3二段階保存 |
| `internal/dataaccess/*.go` | `Money.AmountMinor int64`、`CommandScope`、write port、object lifecycle |
| `internal/database/migrations/000001〜000027` | 現在のテーブル、列、状態、既存データ互換上必要なsnapshot |

未決定の税・為替・承認OTP・棚卸反映・BOX正本等については、既存契約を越える業務ルールを
新たに確定していない。スキーマは値を損失なく保持できる器だけを提供する。

## 3. 成果物

| ファイル | 用途 |
|---|---|
| `deploy/aws/postgres/migrations/000001_initial_schema.up.sql` | 新規空DB用の初期スキーマ候補 |
| `deploy/aws/postgres/migrations/000001_initial_schema.down.sql` | 空のstaging DB専用の破壊的rollback |
| `deploy/aws/postgres/verify_schema.sql` | 適用後のread-only検証 |

`up.sql` はpsql専用で、`ON_ERROR_STOP`、一つのtransaction、advisory lockを利用する。
アプリケーション起動時には実行しない。ECSの単発migration jobからのみ実行する。

## 4. 型と共通規則

### 4.1 IDと業務番号

- 内部IDはprovider-neutral契約に合わせて`TEXT`とし、アプリケーション側で衝突耐性のあるIDを生成する。
- `product_code`、`slip_number`、`request_number`等の業務番号は内部IDから分離する。
- 採番は`business_number_sequences`を同じ業務transaction内で更新する。
- `SELECT ... FOR UPDATE`または原子的な`INSERT ... ON CONFLICT ... DO UPDATE ... RETURNING`を
  PostgreSQL adapter内部で使い、事前の「次番号取得API」は作らない。

### 4.2 金額、通貨、為替

- 金額はすべて最小通貨単位の`BIGINT`とし、浮動小数点を使用しない。
- 金額列は原則`*_minor`、通貨は大文字3文字の`CHAR(3)`で保持する。
- 為替は`rate_scaled BIGINT`と`scale BIGINT`の組で保持する。
- 売上時点の通貨、適用レート、換算額、税区分、税額を伝票snapshotとして保持できる。
- Goでは`int64`、JavaScript/JSON境界では安全整数を越える値を10進文字列として転送する。
- PostgreSQLの`BIGINT`は符号付き64bitであり、`dataaccess.Money.AmountMinor`と損失なく対応する。

### 4.3 日付と日時

- 業務日付は`DATE`。
- イベント日時は`TIMESTAMPTZ`。
- migration job、ECS application、DB sessionは`TimeZone=UTC`とする。
- `TIMESTAMPTZ`は絶対時刻を保持し、Asia/Tokyoへの変換は表示境界のみで行う。
- 文字列timestampやtimezoneなしの`TIMESTAMP`を本番スキーマへ追加しない。

### 4.4 JSONと真偽値

- snapshot、監査before/after、分類固有値は`JSONB`。
- SQLiteの0/1はPostgreSQLでは`BOOLEAN`。
- `JSONB`は自由な外部入力を無検証で保存する許可ではない。adapter/serviceでschema versionと
  allowlistを検証する。

## 5. tenant境界

### 5.1 物理制約

- tenant列は現行`organization_id`を維持し、共通契約上の`TenantID`へadapterで対応させる。
- tenant配下の親テーブルは`UNIQUE (organization_id, id)`を持つ。
- 子テーブルの外部キーは原則
  `FOREIGN KEY (organization_id, parent_id) REFERENCES parent(organization_id, id)`とする。
- これにより、誤った別tenant IDをINSERT/UPDATEしてもDB制約で拒否される。
- 業務番号のUNIQUEもtenant内に限定する。
- tenantを越えた存在確認を外部へ返さず、adapterはmissingとcross-tenantを共に`ErrNotFound`へ畳む。

### 5.2 RLS

初期候補ではRow Level Securityを有効化していない。理由は、現行共通contractに
PostgreSQL sessionへtenant contextを安全に設定するadapterがまだ存在しないためである。
RLSを中途半端に有効化すると、connection poolでtenant contextが漏れる危険がある。

PostgreSQL adapter実装後、次を満たす独立migrationとしてRLSを追加する。

1. transaction開始直後に`SET LOCAL app.tenant_id = ...`を実行する。
2. connection返却後に値が残らないことをcontract testで確認する。
3. owner/bypassrls権限をruntime roleへ付与しない。
4. tenantなし、別tenant、rollback、pool再利用をテストする。

RLS追加前も、すべてのqueryは`organization_id`を必須条件とし、複合外部キーでwrite境界を守る。

## 6. 冪等性、競合、transaction

### 6.1 冪等性

`idempotency_records`の主キーは
`(organization_id, operation_name, idempotency_key)`である。

一つの業務transaction内で次を行う。

1. canonical request hashと冪等性キーを登録する。
2. 同じキーがある場合はrowをlockする。
3. hashが異なれば`ErrIdempotencyMismatch`。
4. 同じhashで`committed`なら保存済み結果を返す。
5. 未処理なら業務変更、在庫イベント、監査ログ、結果を同じtransactionでcommitする。

`processing` rowを先行commitしてはならない。障害時に半端な状態を作らないため、業務変更と
同じtransactionで確定する。長期保持・削除期間は運用決定後に設定し、少なくともクライアントの
最大retry期間より長くする。

### 6.2 CAS

- 更新対象の主要集約は`version BIGINT`を持つ。
- 更新SQLは`WHERE organization_id=$1 AND id=$2 AND version=$expected`とし、
  `version = version + 1`を同時に行う。
- affected rowが0なら、同tenant内で存在確認してNotFoundとConflictを区別する。
- 状態遷移条件も同じ`UPDATE`のWHEREへ含める。

### 6.3 業務transaction

次は各1transactionで完結させる。

- 商品登録: 採番、商品、付属品、BOX、初期在庫イベント、冪等性、監査
- 仕入確定: 伝票、明細、商品、在庫イベント、冪等性、監査
- 売上確定: 伝票、明細、為替snapshot、在庫イベント、冪等性、監査
- 出荷確定: 売上割当、出荷伝票、明細、在庫イベント、冪等性、監査
- 返品在庫戻し: 返品明細、condition、在庫状態、イベント、冪等性、監査

Handlerへ`*sql.Tx`や`pgx.Tx`を公開しない。transaction境界はPostgreSQL repository内部に閉じる。

## 7. 監査と履歴

- `audit_logs`はappend-onlyで、UPDATE/DELETEを拒否するtriggerを持つ。
- runtime DB roleには`audit_logs`のUPDATE/DELETE権限を与えない。
- 業務revision表も原則append-onlyで扱う。初期候補では現行互換を優先し、triggerは監査正本だけに付ける。
- 監査にはtenant、actor、target、action、request ID、idempotency key、before/after、結果、UTC時刻を保存する。
- password、token、cookie、credential、S3署名URL等のsecretを監査JSONへ含めない。
- DB administratorによる変更はRDS Database Activity Streamsまたは承認済み監査方式を
  本番ゲートで別途有効化する。

## 8. index方針

初期候補は現行画面とcontractの主要検索に必要なindexのみを置く。

| 用途 | 主な先頭列 |
|---|---|
| tenant内一覧 | `organization_id` |
| 在庫一覧 | `organization_id, inventory_status, purchase_date DESC, product_code` |
| 伝票一覧 | `organization_id, business_date DESC, business_number` |
| 商品検索 | tenant + `serial_number` / `sku` |
| 購入依頼 | tenant + `request_group_id, status, requested_at DESC` |
| 承認 | tenant + `status, requested_at DESC` |
| 棚卸 | tenant + `stocktake_id, review_status` |
| 監査 | tenant + targetまたはcreated_at |
| S3 reconciliation | `status, created_at` |

PostgreSQLは外部キー列へ自動でindexを作らないため、実query planで必要なFK indexを確認する。
ただし未観測のindexを一括追加するとwrite amplificationとVACUUM負荷が増えるため、stagingの
`pg_stat_statements`と`EXPLAIN (ANALYZE, BUFFERS)`を根拠に追加する。

初期空DBでは通常の`CREATE INDEX`をtransaction内で使う。稼働後の大規模表への追加は、
別migrationで`CREATE INDEX CONCURRENTLY`を使い、transaction外実行と失敗index回収をrunbook化する。

## 9. 現行SQLiteとの差分

| SQLite最終形 | PostgreSQL候補 | adapter/移行時の扱い |
|---|---|---|
| 日付・日時が`TEXT` | `DATE` / `TIMESTAMPTZ` | strict parseし、不正値を事前隔離 |
| 金額が`INTEGER` | `BIGINT` | `int64`範囲を検査 |
| boolが0/1 | `BOOLEAN` | 0=false、1=true。その他はreject |
| JSONが`TEXT` | `JSONB` | JSON parse不能行をreject |
| 単一ID FK | tenant付き複合FK | import時に親子tenant一致を検査 |
| `products.accessories` CSV文字列 | 互換列 + `product_accessories` | 読取互換を維持し、正規化行を生成 |
| `product_images.storage_path` | `product_objects` + S3 private locator | metadataを変換し、bytesはS3へ別移行 |
| `amount_jpy` | `amount_minor` + `currency` | JPYとして変換 |
| customer/destination nameのみ | nullable ID + 表示snapshot | 過去帳票再現のためsnapshotを残す |
| update可能な監査表 | append-only trigger | import後の更新・削除禁止 |

SQLiteの`000023`にあるpreview seedは本番へ移行しない。Cloudflare検証データも本番へ昇格しない。

## 10. S3オブジェクトmetadata

`product_objects`は共通object lifecycleを実装する。

- DBに`pending`を登録
- S3 upload
- HEADでsize/checksumを照合
- DBを`ready`へ更新
- 失敗時は`failed`
- 論理削除時は`deleted`

bucket、key、versionはadapter内部用の列であり、共通DTOや外部APIへ返さない。
S3 bucketはprivate、Block Public Access、SSE-KMS、Versioningを必須とする。
DBとS3は分散transactionを持たないため、reconciliation jobでpending期限切れ、missing、orphan、
checksum不一致、deleted残存を検出する。

## 11. migration artifactの固定

release pipelineは`000001_initial_schema.up.sql`のバイト列からSHA-256を計算し、
次の変数をpsqlへ渡す。

```powershell
$migration = 'deploy/aws/postgres/migrations/000001_initial_schema.up.sql'
$checksum = (Get-FileHash -Algorithm SHA256 -LiteralPath $migration).Hash.ToLowerInvariant()
$executionID = [guid]::NewGuid().ToString()
psql $env:ZAIKO_MIGRATION_DSN `
  --set=ON_ERROR_STOP=1 `
  --set=migration_checksum=$checksum `
  --set=execution_id=$executionID `
  --file=$migration
```

同じversionでchecksumが異なるartifactは適用を拒否する。適用済みmigrationを編集せず、
修正は必ず新しい連番migrationとして追加する。

候補では`schema_migrations.execution_duration_ms`を0で登録する。正式migration runnerでは
実測時間を記録するか、CloudWatchのjob durationを正本としてこの列を廃止する判断を行う。

## 12. RDS migration job手順

### 12.1 task分離

- application taskとmigration taskは別ECS task definitionにする。
- migration taskのdesired countは持たず、deploy pipelineから単発起動する。
- migration imageはapplicationと同じcommit/digest、または署名済み専用imageを使う。
- private subnetからRDSへ接続し、security groupはmigration taskからRDS 5432のみ許可する。
- DSN/passwordはSecrets Managerから注入し、CloudWatch logへ出力しない。
- `sslmode=verify-full`とRDS CA bundleを使用する。
- migration roleだけがschema owner/DDL権限を持ち、runtime roleへDDL、TRIGGER、BYPASSRLSを与えない。

### 12.2 preflight

1. 対象account、region、cluster/instance、database名、release digestを二者確認する。
2. RDS automated backup、PITR、deletion protection、Multi-AZを確認する。
3. 手動snapshotを作成し、availableになるまで待つ。
4. stagingでは対象DBが空であることを確認する。
5. `SELECT version();`、`SHOW server_version_num;`、`SHOW TimeZone;`を記録する。
6. migration user、runtime user、ownerを分離していることを確認する。
7. artifact checksum、Git commit、image digest、execution IDをリリース記録へ固定する。
8. application rolloutを開始していないことを確認する。

### 12.3 apply

1. ECS RunTaskでmigration jobを1 taskだけ起動する。
2. `up.sql`を`ON_ERROR_STOP=1`で実行する。
3. advisory transaction lockにより同時migrationを直列化する。
4. 一つでもDDLが失敗した場合はtransaction全体をrollbackし、jobを非0終了する。
5. job成功後に`verify_schema.sql`をread-only接続で実行する。
6. schema version、checksum、table/constraint/index/trigger、不正なmoney型、tenant列欠落を保存する。
7. migrationと検証が成功した場合だけデータimport jobへ進む。
8. reconciliationが成功した場合だけ新ECS application revisionをrolloutする。

### 12.4 runtime権限

role名はIaCで環境ごとに作るため、候補SQLへ固定名のGRANTを含めていない。適用後にownerが次を行う。

- runtime roleへ`zaiko` schemaのUSAGE
- 必要テーブルのSELECT/INSERT/UPDATE
- `audit_logs`へのINSERTのみ
- `schema_migrations`へのSELECTのみ
- 全テーブルのDELETEは業務上必要なものだけ
- schema CREATE、table DDL、function/trigger変更は不許可
- public schemaへのCREATEはREVOKE

権限matrixはstaging contract testで検証し、migration roleのcredentialをapplication taskへ渡さない。

## 13. SQLiteからの一方向移行

dual-writeは行わない。

1. 現行SQLiteをオンラインのままstaging用snapshotへ複製する。
2. 変換toolで日付、日時、bool、JSON、金額、通貨、tenant親子関係を検証する。
3. 不正行を自動補正せず、reject reportへ出す。
4. PostgreSQL stagingへ親から子の順でCOPYする。
5. tenant別・表別件数を照合する。
6. 金額SUM、業務番号、状態別件数、孤児FK、重複、監査件数を照合する。
7. 商品画像をS3へ移し、size/checksum/HEADを照合して`ready`にする。
8. anonymized stagingでcontract、E2E、負荷、backup/restore、PITRを確認する。
9. 本番maintenance windowでSQLiteをread-onlyにする。
10. final snapshotから同じ変換・import・reconciliationを実行する。
11. 合格後だけECSをPostgreSQL adapter構成で起動し、ALB targetへ追加する。

元SQLiteは暗号化し、rollback判定期間が終わるまで削除しない。

## 14. 検証SQLの判定

`verify_schema.sql`の結果について次を必須とする。

- `schema_migrations`に期待versionとartifact checksumが1件ある。
- tenant対象表の欠落一覧が0件。
- `*_minor`で`BIGINT`以外の一覧が0件。
- 期待するPK、UNIQUE、CHECK、FK、部分UNIQUE indexが存在する。
- `audit_logs_no_update`と`audit_logs_no_delete`が存在する。
- runtime roleでaudit UPDATE/DELETE、DDL、別tenant writeが失敗する。
- migration再実行が「既に適用済み」と安全に終了し、DDLを再適用しない。

現候補のSQLファイルを直接2回実行すると`CREATE SCHEMA`で失敗する。正式runnerは実行前に
`schema_migrations`を確認し、同version・同checksumなら成功済みとして終了し、同version・異checksumなら拒否する。

## 15. 復旧

### 15.1 schema適用中

`up.sql`は単一transactionのため、失敗時は全DDLがrollbackされる。migration jobを失敗終了し、
application rolloutを開始しない。原因修正は同じ適用済みversionを書き換えず、新artifactをレビューする。

### 15.2 空staging DB

データ未投入で、対象が正しい空staging DBであることを確認した場合に限りdown migrationを利用できる。

```powershell
psql $env:ZAIKO_STAGING_MIGRATION_DSN `
  --set=ON_ERROR_STOP=1 `
  --command='SET zaiko.allow_destructive_rollback=on' `
  --file='deploy/aws/postgres/migrations/000001_initial_schema.down.sql'
```

ただしpsqlの`--command`と`--file`が別sessionになる実行方法ではGUCが維持されない。安全のため実運用では
一つのpsql sessionで次を実行する専用rollback scriptを生成し、対象DB名と空データを二者確認する。

```sql
SET zaiko.allow_destructive_rollback = 'on';
\i deploy/aws/postgres/migrations/000001_initial_schema.down.sql
```

### 15.3 データ投入後または本番

down migrationは使用しない。

1. ALBへの新revision登録を止める。
2. writeを停止する。
3. RDS snapshotまたはPITRで別instance/clusterへ復元する。
4. reconciliationを行う。
5. 旧ECS revisionを復元先へ接続する。
6. targetを切り戻す。
7. 失敗DBは証跡として隔離し、即時削除しない。

本番切替直後のapplication障害でDB schemaが後方互換なら旧ECS revisionへ戻す。破壊的schema変更を
同じreleaseへ含めないexpand/contract方式を今後のmigration原則とする。

## 16. 静的検証

この作業環境では`psql`、`pg_dump`、Docker、Podman、Go runtimeを利用できなかったため、
PostgreSQL engineへの実適用は行っていない。実施した検証は次のread-only/static検証に限られる。

- migrationのtransaction境界、advisory lock、psql `ON_ERROR_STOP`の存在
- CREATE TABLE名の重複
- 括弧、単一quote、dollar quoteの対応
- 金額列が`BIGINT`
- 日時列が`TIMESTAMPTZ`
- tenant対象表の`organization_id`
- tenant複合FKと参照先UNIQUEの目視・機械検査
- append-only監査trigger
- version、idempotency、audit、object lifecycleの存在
- down migrationの明示的安全guard
- 指定範囲外に変更がないこと

engine検証未実施のため、本候補を「適用可能」または「本番準備完了」とは判定しない。

## 17. staging合格条件

次がすべて完了するまでAWS本番切替は禁止する。

1. PostgreSQL 16の空DBへup migration適用成功。
2. `verify_schema.sql`の異常一覧が0件。
3. PostgreSQL adapterのread/write contract test合格。
4. tenant越境、NotFound collapse、CAS、冪等性、transaction rollbackの競合試験合格。
5. 最大`int64`境界、全通貨、UTC/JST日付境界の試験合格。
6. SQLite変換・COPY・reconciliationの反復成功。
7. S3 Versioning、checksum、orphan/missing recovery試験合格。
8. load testとquery plan監査合格。
9. RDS snapshot restoreとPITRリハーサル合格。
10. migration job失敗時にapplication rolloutが開始されないこと。
11. 監視、alert、責任者、RTO/RPO、read-only window、切戻し判定の承認。

## 18. 次工程TODO

- PostgreSQL 16 stagingでの実DDL検証
- PostgreSQL adapterとcontract test
- migration manifest/runner
- SQLite→PostgreSQL変換・一方向import tool
- runtime/migration DB roleのIaC
- tenant RLS追加migrationとpool安全性試験
- S3 adapterとreconciliation job
- 税・為替・承認・棚卸・BOXの未決定事項確定後の追加migration
- load/PITR/restore/cutoverリハーサル
