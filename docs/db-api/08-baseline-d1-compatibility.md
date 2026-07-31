# 000001〜000027 baseline候補・D1互換性検証

## 1. 結論

`baseline-candidate-000027.sql` を、現行 up migration の追跡可能な静的合成候補として作成した。

- 既存 `000001`〜`000027` は変更していない。
- 現行DB、本番DB、Cloudflare remote resourceには適用していない。
- `000023_phase8_sales_destinations` はスキーマ変更を含まず、`org_preview` 専用データの DELETE / INSERT / UPDATE だけであるためbaseline候補から除外した。
- 残る26ファイルを番号順に合成した。
- D1では変更できない `PRAGMA foreign_keys = ON` を候補から除外した。
- 明示的な `BEGIN TRANSACTION` / `COMMIT` は追加していない。
- `000020` の `PRAGMA defer_foreign_keys = ON` は維持した。

ただし、現在の実行環境にはGo SDK、SQLite CLI、Python、Node、Wranglerがなく、空DBへの実適用は完了していない。この候補を「D1互換確認済み」または「deploy可能な最終baseline」と扱ってはならない。

現時点の判定は次のとおり。

| 判定 | 結果 |
|---|---|
| 静的合成 | 完了 |
| legacy sourceの追跡 | 完了 |
| preview seed除外 | 完了 |
| SQLサイズ静的確認 | 完了 |
| ローカルSQLite空DB適用 | 未実施（CLI/SDKなし） |
| Wrangler local D1適用 | 未実施（Node/Wranglerなし） |
| Remote D1適用 | 意図的に未実施 |
| deploy可能なflat baseline化 | 未実施 |

## 2. 成果物

- `docs/db-api/baseline-candidate-000027.sql`
  - legacy migration境界コメント付き
  - 約45 KB
  - 26 source section
  - preview seedなし
- 本書

候補SQLは、空DBに対してlegacy DDLを順次再生する「replay形式」である。最終的なD1 baselineは、空DB検証合格後にALTER、交換レート一時テーブル、データ補正UPDATEを最終CREATE定義へ畳み込んだflat形式にする。

## 3. ランタイム探索結果

2026-07-31、PowerShellの `Get-Command` と既知パスを確認した。

| コマンド | 結果 |
|---|---|
| `go` | NOT_FOUND |
| `gofmt` | NOT_FOUND |
| `sqlite3` | NOT_FOUND |
| `python`, `python3`, `py` | NOT_FOUND |
| `node`, `npm` | NOT_FOUND |
| `bun`, `deno` | NOT_FOUND |
| `Microsoft.Data.Sqlite` .NET assembly | NOT_FOUND |
| `System.Data.SQLite` .NET assembly | NOT_FOUND |
| `Mono.Data.Sqlite` .NET assembly | NOT_FOUND |

Codexワークスペース依存ランタイムの取得も試みたが、ツール待機中に中断されたため利用できなかった。

## 4. 静的検証結果

### 4.1 構成

| 項目 | 結果 |
|---|---:|
| ファイルサイズ | 45,214 bytes |
| legacy source section | 26 |
| `CREATE TABLE` | 50 |
| 最終想定テーブル | 49 |
| 一意な索引名 | 47 |
| trigger | 2 |
| `ALTER TABLE` | 67 |
| `PRAGMA defer_foreign_keys` | 1 |
| `PRAGMA foreign_keys` | 0 |
| 明示的transaction | 0 |
| preview ID参照 | 0 |
| セミコロン単純分割時の最大断片 | 1,411文字 |

`CREATE TABLE` 50件には `exchange_rate_snapshots_phase8` が含まれる。このテーブルは `000020` で最終的に `exchange_rate_snapshots` へrenameされ、旧テーブルを置換するため、適用後の想定テーブル数は49である。D1のmigration管理テーブルはこの数に含めていない。

最大断片はコメントを含む単純分割値であり、正規SQL parserによる厳密なstatement lengthではない。それでも、D1の1 statement 100 KB制限に対して十分小さい候補である。

### 4.2 既存テストにある根拠

現行リポジトリには、Go/modernc SQLiteが利用可能な環境で次を検証する既存テストがある。

- `internal/database/database_test.go:144-199`
  - phase5相当DBへ残りmigrationを適用
  - データ件数維持
  - 再度 `Migrate` して冪等性確認
  - migration 27件と最終版 `000027_purchase_request_groups` を確認
- `internal/database/database_test.go:201-301`
  - `000020` の交換レート再構築
  - legacy参照の切離し
  - captured rate維持
  - `pragma_foreign_key_check` 0件

これらは過去のローカルSQLite互換性の根拠にはなるが、この作業環境では再実行できておらず、D1互換性の証明でもない。

## 5. D1公式仕様との照合

### 5.1 外部キー

D1は外部キーを常時強制し、各query/migrationを暗黙transaction内で実行するため、利用者SQLから `PRAGMA foreign_keys = off` へ変更できない。代わりに `PRAGMA defer_foreign_keys = on` がmigration用に提供される。

根拠:

- [Cloudflare D1: Define foreign keys](https://developers.cloudflare.com/d1/sql-api/foreign-keys/)
- [Cloudflare D1: Migrations](https://developers.cloudflare.com/d1/reference/migrations/)

対応:

- 元の `000001` にある `PRAGMA foreign_keys = ON` はbaseline候補から除外した。
- `internal/database/database.go` が `000020` 適用時に実行する `PRAGMA foreign_keys = OFF/ON` はD1 adapterへ移植できない。
- `000020` 自身の `PRAGMA defer_foreign_keys = ON` はD1の推奨方式に一致するため候補に残した。

### 5.2 明示的transaction

D1 importで `BEGIN TRANSACTION` / `COMMIT` を含むと、暗黙transactionとの重複により失敗する場合がある。候補には明示的transactionを含めていない。

根拠:

- [Cloudflare D1: Import and export data](https://developers.cloudflare.com/d1/best-practices/import-export-data/)

### 5.3 SQLとPRAGMA

D1はSQLite query engineを利用し、SQLite SQLの多くと `PRAGMA defer_foreign_keys`、`PRAGMA foreign_key_check` をサポートする。

根拠:

- [Cloudflare D1: SQL statements](https://developers.cloudflare.com/d1/sql-api/sql-statements/)

ただし「SQLite SQLの多くをサポート」は、この候補の全statementが成功する保証ではない。特に以下はWrangler localで実行確認が必要である。

- partial UNIQUE index
- audit logのUPDATE/DELETE禁止trigger
- `ALTER TABLE ... ADD COLUMN ... REFERENCES`
- `ALTER TABLE ... ADD COLUMN ... CHECK`
- `DROP TABLE` と `ALTER TABLE ... RENAME TO` を使う `000020`
- 同じmigration中の `PRAGMA defer_foreign_keys`

### 5.4 D1制限

候補ファイル約45 KB、最大statement断片約1.4 KBは、D1の最大SQL statement 100 KBより小さい。現在のテーブル列数も100列制限内と見込まれるが、実適用後の `table_info` 検証を必須とする。

根拠:

- [Cloudflare D1: Limits](https://developers.cloudflare.com/d1/platform/limits/)

データ移行時は、次の制限も別途考慮する。

- bound parameters 100
- query duration 30秒
- 1行/string/BLOB 2 MB
- JavaScript境界での52 bit整数精度
- 大規模UPDATE/DELETEのbatch分割

## 6. 非互換・危険SQL一覧

### 6.1 D1へ移植不可

| 対象 | 判定 | 対応 |
|---|---|---|
| `internal/database/database.go` の `PRAGMA foreign_keys = OFF` | D1では変更不可 | D1 migration内で `PRAGMA defer_foreign_keys = ON` を使う |
| 同ファイルの `PRAGMA journal_mode = WAL` | D1管理環境へ適用しない | D1 adapterの接続初期化から除外 |
| 同ファイルの `PRAGMA busy_timeout = 5000` | ローカル接続用。D1 HTTP/bindingへ適用しない | retry/backoffとD1 error分類へ置換 |

上記は候補SQLには含まれていない。

### 6.2 構文上は候補だが未検証

| SQL | リスク |
|---|---|
| `PRAGMA defer_foreign_keys = ON` | D1対応済みだが、migration全体の適用単位をWrangler localで確認する必要がある |
| 旧 `exchange_rate_snapshots` のDROP/一時表rename | 参照元 `sales_lines` を含むため、空DBと既存データありの両方で確認が必要 |
| 67件の `ALTER TABLE ADD COLUMN` | replay形式のため不要に複雑。flat baselineではCREATEへ統合する |
| partial UNIQUE indexes | 対象状態の排他に重要。作成失敗時に通常indexへ弱めてはならない |
| audit triggers | 作成とUPDATE/DELETE拒否の動作試験が必要 |
| データ補正UPDATE 3件 | 空DBではno-op。既存データ移行とは別手順へ分離すべき |

### 6.3 baselineから除外したSQL

`000023_phase8_sales_destinations.up.sql` はすべて `org_preview` 用のデータ操作である。production baselineへpreviewデータを混入させないため除外した。

## 7. 再現可能な検証手順

### 7.1 ローカルSQLite CLI

前提:

- SQLite 3 CLIがPATHにある。
- 現行DBではなく、新規一時DBを使う。

PowerShell:

```powershell
$candidate = (Resolve-Path 'docs/db-api/baseline-candidate-000027.sql').Path
$scratch = Join-Path ([System.IO.Path]::GetTempPath()) ('zaiko-baseline-' + [guid]::NewGuid().ToString('N') + '.db')

sqlite3.exe $scratch '.bail on' ('.read "' + $candidate.Replace('\', '/') + '"')
sqlite3.exe $scratch 'PRAGMA integrity_check;'
sqlite3.exe $scratch 'PRAGMA foreign_key_check;'
sqlite3.exe $scratch "SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%';"
sqlite3.exe $scratch "SELECT COUNT(DISTINCT name) FROM sqlite_schema WHERE type='index' AND sql IS NOT NULL;"
sqlite3.exe $scratch "SELECT COUNT(*) FROM sqlite_schema WHERE type='trigger';"
sqlite3.exe $scratch "SELECT name FROM sqlite_schema WHERE type='table' ORDER BY name;"
```

期待値:

- `integrity_check`: `ok`
- `foreign_key_check`: 0行
- 業務テーブル: 49
- 明示索引名: 47
- trigger: 2
- `exchange_rate_snapshots_phase8` が残っていない
- `exchange_rate_snapshots` が存在する

検証後は、`$scratch` が一時ディレクトリ内の今回作成ファイルであることを確認してから削除する。現行DBパスを変数へ設定しない。

### 7.2 既存Go migration test

Go SDKが利用可能な環境:

```powershell
go test ./internal/database -run 'TestMigrateExistingPhase5DatabasePreservesData|TestPhase8RateMigrationUpgradesReferencedLegacySnapshots' -count=1
go test ./internal/database -count=1
```

これは既存 `Store.Migrate` の回帰検証であり、candidate SQL自体のD1検証ではない。

### 7.3 Wrangler local D1

Node、Wrangler、ローカル専用D1設定を準備した後、必ず `--local` で実行する。

```powershell
npx wrangler d1 execute <LOCAL_TEST_BINDING> --local --file docs/db-api/baseline-candidate-000027.sql
npx wrangler d1 execute <LOCAL_TEST_BINDING> --local --command 'PRAGMA foreign_key_check;'
npx wrangler d1 execute <LOCAL_TEST_BINDING> --local --command "SELECT type,name FROM sqlite_schema WHERE type IN ('table','index','trigger') ORDER BY type,name;"
```

禁止:

- `--remote`
- production database name / ID
- 現行DBファイルの指定
- 検証未完のcandidateをmigration directoryへ配置すること

### 7.4 D1 migration化前の追加試験

1. 空DBへcandidateを一括適用する。
2. 空DBへ26 sectionを個別順次適用する。
3. `000019` 相当までのfixtureを作り、`000020` sectionを適用する。
4. 有効なUSD/JPY rateが保持されることを確認する。
5. legacy JPY/USD rateの親IDだけがNULLになり、captured rateが維持されることを確認する。
6. partial UNIQUE indexへ競合行を投入し拒否されることを確認する。
7. audit_logsのUPDATE/DELETEがtriggerで拒否されることを確認する。
8. 全外部キー、索引、列、CHECK定義をSQLite最終形と比較する。

## 8. deploy可能なflat baselineへの変換方針

実行検証合格後、別ファイルとしてflat baselineを作る。

1. 全 `ALTER TABLE ADD COLUMN` を各最終 `CREATE TABLE` へ統合する。
2. `exchange_rate_snapshots` はphase8後の最終定義だけを作成する。
3. `exchange_rate_snapshots_phase8`、コピーINSERT、detach UPDATE、DROP、renameを除去する。
4. 空DBでno-opとなるstocktake、purchase request等のUPDATEを除去する。
5. preview seedは含めない。
6. 最終索引47件、trigger 2件を一度だけ作成する。
7. D1 migration管理はWranglerに任せ、現行 `schema_migrations` をbaselineへ追加しない。
8. flat版とreplay版を別々の空SQLite/D1 localへ適用し、`sqlite_schema` を正規化比較する。

この変換は現行 migrationの改変ではなく、新規D1 bootstrap artifactとして管理する。

## 9. 復旧方法

### 9.1 ローカル検証

- 検証DBは毎回一意な一時ファイルとして作る。
- 失敗時はログと失敗sectionを保存し、一時DBを破棄して最初から再実行する。
- candidateは冪等適用を保証していないため、失敗DBへ途中から再適用しない。
- 現行DBをコピー元として使用する場合も、原本を読み取り専用で保全する。

### 9.2 D1 local

- local database stateを破棄し、新しいlocal stateへ再適用する。
- migration成功判定前にcandidateを正式migration履歴へ入れない。

### 9.3 将来のremote検証

この作業ではremoteへ適用しない。将来実施する場合は次を必須とする。

- productionと別account/database
- Time Travel bookmarkまたはexport
- migration前後のschema export
- foreign key、件数、金額、テナント境界の照合
- 失敗時はTime Travel restoreまたは検証DB廃棄
- 既存migrationのdown実行を復旧手段としない

`000012`〜`000018` はdown migrationがないため、production復旧はforward fixまたはバックアップ/Time Travel復元を基本とする。

## 10. 残作業と合格条件

残作業:

- SQLite CLIまたはGo SDKを用いた空DB実適用
- Wrangler local D1実適用
- `000020` のデータありmigration試験
- schema正規化比較
- flat baseline生成
- repository/adapter契約テスト

合格条件:

- SQLite空DBとD1 localの両方で適用成功
- `integrity_check=ok`
- `foreign_key_check` 0件
- 最終テーブル49、明示索引47、trigger 2
- 一時テーブル、preview seed、明示transactionなし
- partial UNIQUEとaudit triggerの挙動合格
- `000020` の既存データ保持試験合格
- flat版とreplay版の最終schema一致
- remote resource未使用のレビュー記録

この条件を満たすまでは、`baseline-candidate-000027.sql` を候補のまま維持する。
