# 設定項目

## 起動環境変数

| 変数 | 開発初期値 | 本番要件 |
|---|---|---|
| `ZAIKO_ADDRESS` | `127.0.0.1:8080` | リバースプロキシ配下の待受 |
| `ZAIKO_DATABASE_PATH` | `.data/zaiko.db` | 絶対パス、永続ディスク |
| `ZAIKO_UPLOAD_DIRECTORY` | `.data/uploads` | 絶対パス、バックアップ対象 |
| `ZAIKO_ENV` | `development` | `production` |
| `ZAIKO_COOKIE_SECURE` | `false` | `true`必須 |
| `ZAIKO_SESSION_TTL` | `12h` | 5分〜168時間 |
| `ZAIKO_ORGANIZATION_CODE` | `PREVIEW` | 初期管理者作成時の組織コード |
| `ZAIKO_PREVIEW_ADMIN_PASSWORD` | プレビュー値 | 開発環境だけで使用 |
| `ZAIKO_PREVIEW_WORKER_PASSWORD` | プレビュー値 | 開発環境だけで使用 |

## 組織設定

| 設定 | 初期状態 | 運用判断 |
|---|---|---|
| 仕入承認金額 | 未設定 | 承認対象とする円金額 |
| 売上承認金額 | 未設定 | 作業者の承認対象とする円金額 |
| 管理者高額承認モード | 無効 | 複数管理者体制で有効化 |
| 管理者高額承認金額 | 未設定 | モード有効化前に設定 |
| 取置期限 | 未設定 | 未設定時は24時間 |
| 為替レート取得方法 | `manual` | 現状は手入力またはCSV |
| CSV文字コード | `UTF-8-BOM` | 連携先に合わせて選択 |

承認閾値と取置期限は業務責任者、CSVと為替設定はシステム管理者が本番利用開始前に確定してください。
