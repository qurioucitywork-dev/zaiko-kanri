# 目標アーキテクチャと段階移行メモ

データ項目ごとの正本、RDS/S3/ブラウザの保存境界、現状との差分は
[データ保存先・所有権設計](data-storage-ownership.md)を参照する。

## 目標

- フロントエンド: React + JavaScript（Vite）
- バックエンド: Go
- ORM: GORM
- API: REST API `/api/v1`
- 認証: HttpOnly Cookie Session + CSRF（将来JWTへ切替可能）
- DB: PostgreSQL / Amazon RDS for PostgreSQL
- ファイル: Amazon S3
- 実行基盤: Amazon ECS on AWS Fargate

Cookie Sessionを選んだ理由は、既存のサーバーセッション、権限、CSRF、強制失効をそのまま維持できるため。JWTが必要になるのは、外部クライアントや別ドメインAPIを提供するときに限定する。

## 2026-08-03時点の実装状況

| 項目 | 状態 | 現在の代替・接続先 | 本番化までの残作業 |
|---|---|---|---|
| React | 接続済み | `/app/`を単一入口とし、基準HTMLをReactから同一DOMへ直接マウント。iframeなしでREST APIへ接続 | 基準デザインを保ったまま業務ハンドラをReactコンポーネントへ段階移行、E2Eを拡充 |
| REST API | 主要業務接続済み | 認証、マスタ、取引先、相場、仕入、商品、在庫、BOX、購入依頼、承認、通知、出荷、売上、返品、追跡、CSV、帳票履歴、集計 | OpenAPIとE2E契約テストを拡充 |
| 相場表 | 接続済み | CSV取込・詳細行編集・マスタ参照・JPY/USD表示をPostgreSQLへ保存 | 元CSVをS3保管する場合の方針確定 |
| 在庫売価USD | 接続済み | 原通貨、確定時換算額、為替スナップショットをDB保持 | 既存本番データ移行時の差額照合 |
| GORM | 接続済み | PostgreSQLの読取・書込・確定トランザクション | 補助操作の監査網羅 |
| PostgreSQL | ローカル稼働中 | WSL2 PostgreSQL 18.4、migration `000001`～`000016` | RDS作成、移行リハーサル、バックアップ復元試験 |
| Cookie Session | 接続済み | HttpOnly/SameSite Cookie + CSRF + DB session | 本番Cookie設定、必要時のみRedisを検討 |
| ログイン情報管理 | 接続済み | bcrypt、利用者・仕入担当者・ゲスト・取引先コードをPostgreSQLへ統合 | 招待・再設定メールのSES配送 |
| S3 | アダプター実装済み | 既定はローカルディスク | AWS認証、暗号化、ライフサイクル、署名URL方針 |
| ECS Fargate | 定義済み | Dockerfile + Terraform（serviceは既定で無効） | ECRイメージ、DNS/ACM、デプロイパイプライン |
| RDS | 定義済み | Terraform | バックアップ、Multi-AZ、監視、移行リハーサル |

## 参照管理画面の一致作業

2026-08-01に指定されたGenspark管理画面は、見た目と操作の回帰基準として`frontend/public/admin-reference`へスナップショットしている。2026-08-03から通常の`/app/`はこの基準画面をReact入口から同一DOMへ直接マウントする。iframeおよび別デザインのReact画面は使用しない。`APP_DATA`は画面描画用の一時投影として残るが、起動時にGo REST APIから再生成され、API接続時の主要更新はPostgreSQLへ保存される。

## 相場表の本番移行メモ

相場表は`market_price_records`と詳細マスタ参照、`market_price_accessories`へ保存する。CSVはサーバー側で全行検証してプレビュー後に確定し、行編集ポップアップも`PATCH /api/v1/market-prices/{id}`へ接続する。

現在の実装と本番移行で維持する原則は次の通り。

1. PostgreSQLの`market_price_records`へ組織ID、取り込み日、仕入れ価格、相場価格、通貨、取込元、作成者・更新者を保持する。
2. `GET/POST/PATCH /api/v1/market-prices`とCSVプレビュー・確定APIをGORM repository経由で使用する。
3. CSVは文字コード、必須列、数値、日付をサーバー側でも再検証し、行単位の成功・失敗理由を返す。重複判定キーと上書き方針は運用開始前に確定する。
4. 元CSVを保管する場合はS3へ暗号化保存し、RDSにはobject keyと取込結果だけを保存する。
5. 相場価格の通貨はUSD固定で開始し、将来の複数通貨対応時は`currency_code`を追加する。為替換算値と原値を混在させない。
6. 価格変更とCSV取込は監査ログへ変更前後、操作者、取込ファイル、日時を記録する。

## 売価USD統一の本番移行メモ

ローカルの既存在庫・売上・出荷サンプルは、画面内で管理していた固定レート `1 USD = 155 JPY` を使って整数USDへ丸めている。
仕入金額と原価はJPYのまま保持し、売価、販売金額、売上合計、返品売価、出荷卸値、売価由来の粗利だけをUSD表示する。

本番移行では固定レート換算を再利用せず、元の金額・元通貨、USD換算額、適用した為替スナップショットID、換算日時を保持する。
既存RDSデータの移行前には対象件数と換算差額をCSVで照合し、監査ログ付きの一括移行として実行すること。

## 現在の互換レイヤー

旧`internal/database`とSQLiteは旧Go画面・単体テスト互換用に残す。通常画面のAPIブリッジは`internal/persistence`のPostgreSQL実装を正本として利用する。ローカルとAWSで変えるのはDB接続文字列とストレージドライバであり、フロントのAPI契約と業務repositoryは共通である。

## ログイン情報管理の本番移行メモ

マスタ登録の「パスワード管理」は、管理者・作業者・ゲスト、仕入担当者コード、販売先コードをPostgreSQLの`users`、`staff_profiles`、`guest_accounts`、`partner_roles`へ統合している。作業者向け仕入担当者APIはログインIDやメールを返さず、必要な固定コードと表示名だけを公開する。

本番運用で維持する原則は次の通り。

1. PostgreSQLに`users`、`guest_accounts`、`customers`、`user_customer_links`を用意し、テナントID、状態、最終ログイン日時、作成者・更新者、論理削除日時を保持する。
2. パスワード平文をフロントエンド、ログ、RDSへ保存しない。Go側でArgon2idまたはbcryptを使ってハッシュ化し、再設定用の短時間・一回限りトークンを採用する。
3. 管理APIは`GET/POST/PATCH /api/v1/admin/login-accounts`と顧客情報APIへ分離し、管理者ロールとCSRF検証を必須にする。
4. アカウント作成、ログインID変更、権限変更、停止、パスワード再設定を監査ログへ記録し、変更前後と操作者を追跡可能にする。
5. 管理者本人の最終1件は停止できない、現在のセッションを失効させる変更は再認証を要求する、重複IDはDBの一意制約でも拒否する。
6. 初期パスワードを画面やメール本文で配布せず、期限付きの招待リンクから本人が設定する。メール送信失敗時の再送と失効操作を用意する。

## 残作業の順序

1. PostgreSQL接続の管理者・作業者・ゲスト業務シナリオをE2Eテストへ固定する。
2. SESメール配送とPDF確定版の生成・再取得保管を実装する。
3. 商品画像とPDFをS3へ実接続し、署名URL、容量制限、削除整合性を確認する。
4. 本番相当データでRDS移行リハーサルを行い、件数・金額・監査ログを照合する。
5. ECS Fargate、RDS、S3へ接続し、バックアップ・監視・復旧を確認する。
6. 基準デザインとの画素差を出さない単位で、互換JavaScriptをReactコンポーネントへ段階移行する。
7. 安定運用後にSQLite互換レイヤーを段階的に削除する。

## 本番切替の必須条件

- PostgreSQLで全テストが成功すること
- RDSの自動バックアップ、削除保護、暗号化、Performance Insightsを確認すること
- S3 Block Public Access、暗号化、CORS、ライフサイクルを確認すること
- ECSタスクがprivate subnetで実行され、ALBだけが公開されること
- Secrets ManagerからDB接続情報を注入し、平文をtask definitionへ書かないこと
- HTTPS（ACM）と `ZAIKO_COOKIE_SECURE=true` を有効化すること
- 監査ログの更新・削除不可をPostgreSQLでも保証すること
- ロールバック手順とメンテナンス時間を確定すること

## ローカル開発

PostgreSQLモードを標準確認に使用する。

```powershell
.\scripts\dev-postgres.ps1
```

Reactを変更した場合:

```powershell
cd frontend
pnpm install
pnpm run build
cd ..
.\scripts\dev.ps1
```

SQLite互換モードが必要な旧画面確認だけは`.\scripts\dev.ps1`を使用する。業務フロー確認は`.\scripts\dev-postgres.ps1`を使用する。
