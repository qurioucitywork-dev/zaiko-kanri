# AWS実行時コンポジション

`internal/platform/awsruntime` は、最終本番先であるAWS向けの永続化依存を
明示的に組み立てる境界です。

- RDS PostgreSQL: `postgresdb`（pgx / `database/sql`）
- データアクセス: `postgresadapter`
- 認証: ECSタスクロールを標準AWS credential chainから取得
- オブジェクト: private S3 + `s3blob`
- 画像整合性: `objectservice` の pending → PUT → HEAD → ready

このパッケージはマイグレーション、シード、バケット作成、SQLiteとの
dual-writeを行いません。現行WebサーバーのSQLite依存も自動では切り替えません。
全write port、認証・セッション、実PostgreSQL結合試験が揃った後、画面単位の
段階的なcomposition切替で利用します。

必要な実行時設定は次のとおりです。

- PostgreSQL DSN: Secrets ManagerからECS secretとして注入
- AWS region / S3 bucket / object prefix: IaCまたはSSMから注入
- AWS認証情報: 静的キーを注入せず、task roleを使用
- PostgreSQL本番TLS: `sslmode=verify-full`
- S3 endpoint: 省略時はリージョナルAWS endpoint

`Open` はPostgreSQLへの読み取り可能性を検査しますが、S3へテストオブジェクトを
書き込みません。S3権限の実地確認は隔離staging bucketで行い、本番bucketへ
テストデータを保存しないでください。
