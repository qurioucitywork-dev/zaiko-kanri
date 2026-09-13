# ローカルPostgreSQL

## 現在の構成

- PostgreSQL 18.4をWSL2の`Ubuntu`へインストール済み
- 接続先: `127.0.0.1:5432`
- データベース: `zaiko`
- 開発ユーザー: `zaiko`
- 開発用パスワード: `zaiko-local`
- 認証方式: `SCRAM-SHA-256`
- データ実体: WSL2内の`/var/lib/postgresql/18/main`

この認証情報はローカル開発専用です。本番環境では使用せず、RDSの認証情報をAWS Secrets Managerから注入します。

## 起動・確認・停止

プロジェクト用スクリプトがWSL2内のPostgreSQLクラスタを起動し、軽量な維持プロセスでWSLを稼働させ、Windows側の`localhost:5432`から接続できることを確認します。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\local-postgres.ps1 start
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\local-postgres.ps1 status
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\local-postgres.ps1 stop
```

接続文字列は[`.env.postgres.example`](../.env.postgres.example)を参照してください。

## 現時点でPostgreSQLへ保存される範囲

通常の管理者・作業者・ゲスト画面はGo REST APIを通じ、次のデータをPostgreSQLへ保存します。

- 組織・会社情報・振込先・設定・為替レート履歴
- 管理者・作業者・仕入担当者・ゲストアカウント・Cookie Session
- ブランド、素材、駆動方式、コンディション、付属品
- 販売先・仕入先・取引先会社と共通固定コード
- 相場表とCSV取込履歴
- 仕入伝票、商品単品登録、在庫、画像メタデータ
- BOX、公開先、ゲスト向けライブカタログ、購入リクエスト、取置
- 承認申請、通知、既読状態
- 出荷、売上、売上返品、持ち帰り、仕入返品、統合伝票一覧
- 出荷・返品の配送会社と追跡番号
- 在庫・相場・仕入・売上・出荷・返品・棚卸・伝票履歴CSVの発行履歴
- ダッシュボードの確定伝票集計、監査ログ、メール送信待ちキュー

通常画面はReact入口から基準デザインを同一DOMへ表示し、Go REST APIから取得した状態で`APP_DATA`投影を再生成します。互換用のブラウザ保存は残っていますが、API接続時の業務データの正本ではありません。再読込・別端末共有の正本はPostgreSQLです。

PostgreSQLモードの起動:

```powershell
.\scripts\dev-postgres.ps1
```

残る段階移行は、メール配送、PDFファイル生成・保管、S3/RDS/ECSへの実接続です。

データの保存先と責務の全体像は[`data-storage-ownership.md`](./data-storage-ownership.md)を参照してください。

## PostgreSQL導入で対応できること／追加実装が必要なこと

| 課題 | 対応可否 | PostgreSQL以外に必要な実装 |
|---|---|---|
| 仕入・在庫・売上・出荷・返品・相場表の永続化 | 接続済み | 補助項目を含む回帰試験を継続 |
| 管理者・作業者の承認と通知共有 | 接続済み | メール等の外部通知を追加予定 |
| BOX公開・ゲスト購入リクエストの端末間共有 | 接続済み | 同一Go APIへ接続する端末間で共有 |
| Cookie Session認証とパスワード管理の統一 | 接続済み | 本番の招待・再設定フローを追加予定 |
| 出荷済・売上済商品のゲスト非表示 | 接続済み | ライブカタログで現在の在庫状態を照合 |
| 商品画像の保存 | ローカル接続済み | S3ドライバへ切替後に実AWSで確認 |
| 為替・売上目標・仕入予算・レート履歴 | 接続済み | 外部為替レート自動取得は未実装 |
| メール送信 | キューまで接続済み | SES配送ワーカー、再送、失敗監視 |
| 印刷・ダウンロード | 主要帳票・業務CSV・発行履歴まで接続済み | PDFファイル生成・再取得保管は追加予定 |
| 日付の固定表示 | 修正済み | Asia/Tokyoの現在日付を表示 |
| 監査ログ | 主要APIで接続済み | 全補助操作への網羅を継続 |
| RDS・S3・ECS Fargate | 定義済み | AWS適用、Secrets、IAM、ネットワーク、CI/CD |

ローカルPostgreSQLは「共有される正本」を作るための土台です。別PC間で実際に共有するには、各端末が同じGo APIへ接続する必要があります。本番ではECS上のAPIとRDSを共通接続先にします。
