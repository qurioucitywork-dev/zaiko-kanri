# PostgreSQL実行時接続

AWS本番向けの接続境界を `internal/platform/postgresdb` に追加した。現行
SQLiteの `internal/database.Store` と `cmd/server` はまだ切り替えていない。

## 接続条件

- `github.com/jackc/pgx/v5/stdlib` を `database/sql` ドライバーとして使用する。
- DSNは `ZAIKO_POSTGRES_DSN` から受け取り、ログへ出力しない。
- 本番ではURL形式のDSNと `sslmode=verify-full` を必須にする。
- session timezoneはUTCに固定する。
- pool上限、idle上限、connection lifetimeはcomposition rootから指定する。
- 起動時に期限付き `PingContext` を行う。
- schema migration、seed、repairは接続処理から実行しない。

## 読取専用診断

`cmd/pgdiag` は接続性と `zaiko.schema_migrations` の存在だけを確認する。
リモート資源を作成・更新しない。

```powershell
$env:ZAIKO_POSTGRES_DSN = 'postgresql://.../zaiko?sslmode=verify-full'
$env:ZAIKO_S3_BUCKET = 'private-bucket-name'
$env:ZAIKO_S3_PREFIX = 'products'
go run ./cmd/pgdiag
```

## 切替前の残作業

- PostgreSQL側で全ての業務write portを完成させる。
- 認証、承認、棚卸、公開snapshotをprovider-neutral化する。
- 匿名化staging RDSでmigration、contract、負荷、failover、PITRを確認する。
- S3 task-role signerとreconciliation jobを組み込む。
- 上記合格後にのみ `cmd/server` のcomposition rootを切り替える。
