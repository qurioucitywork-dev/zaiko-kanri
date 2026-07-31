# SQLite移行エクスポート

`cmd/dbexport` は、現行SQLiteを変更せずにAWS PostgreSQL移行用の証跡を作るための読取専用ツールである。

## 安全性

- 入力DBはSQLiteの `mode=ro` と `query_only=1` で開く。
- エクスポート全体を1つの読取トランザクションで取得する。
- 出力先は「存在しない新規ディレクトリ」に限定し、既存証跡を上書きしない。
- 各テーブルを主キー順（主キーがなければ全列順）でNDJSONへ出力する。
- 64bit整数はJSON numberではなく、型付き文字列として保存する。
- 各テーブルのSHA-256、行数、列、主キー、スキーマバージョンをmanifestへ記録する。
- 接続先、認証情報、リモートDB、RDS、D1、R2、S3は操作しない。

## 実行例

出力先は必ず新しいディレクトリ名にする。

```powershell
go run ./cmd/dbexport `
  -db 'C:\path\to\zaiko.db' `
  -out 'C:\secure-migration\export-20260731'
```

生成物:

- `manifest.json`
- `<table>.ndjson`

生成後は、DBや外部サービスへ接続しない検証コマンドで、行数・型・
SHA-256・ファイル名・テーブル/列構造を再検証する。

```powershell
go run ./cmd/dbverify `
  -artifact 'C:\secure-migration\export-20260731'
```

この成果物は直接PostgreSQLへ投入しない。変換・検証・reject report・件数/金額/参照整合性reconciliationを行い、匿名化stagingで合格した後にだけ一方向importへ使用する。
