# 在庫管理システム

時計・貴金属等の仕入、在庫、相場、売上、出荷、購入依頼、承認、監査を一元管理するReact + Go製業務システムです。

## 新アーキテクチャへの移行状況（2026-08-03）

- React + JavaScript: `/app/`を単一のReact入口へ統一し、指定された基準デザインを同一DOMへ直接マウント。iframeと別デザイン画面を廃止
- Go REST API: 認証、マスタ、取引先、相場、仕入、商品、在庫、BOX、購入依頼、承認、通知、出荷、売上、返品、伝票、追跡番号、CSV、帳票履歴、集計を接続済み
- GORM: PostgreSQLの読取・書込・トランザクションで利用
- PostgreSQL: WSL2上のローカルDBを業務データの正本として接続済み。migration `000001`～`000016`を適用
- Amazon RDS: Terraform定義済み。実AWSへの作成・データ移行は未実施
- Amazon S3: ローカル/S3切替ストレージと商品画像APIを接続済み。現在のローカル起動はローカルファイル保存
- Amazon ECS Fargate: DockerfileとTerraformを準備済み。実デプロイは未実施
- 認証: bcrypt + HttpOnly Cookie Session + CSRF + API権限制御

通常の管理画面は`http://127.0.0.1:8080/app/`です。8080番を使用中なら`-Port 18086`で起動し、`http://127.0.0.1:18086/app/`を開きます。管理者・作業者・ゲスト画面は、React入口から基準デザインをiframeなしで表示し、画面内のAPIブリッジがGo REST APIへ直接接続します。主要業務データの正本はPostgreSQLです。詳細と本番切替条件は[目標アーキテクチャと段階移行メモ](docs/target-architecture.md)を参照してください。

## 現在の実装

フェーズ1〜8として次の機能を実装しています。

- 組織スコープ
- 管理者・作業者ロールとサーバー側権限判定
- bcryptパスワード認証と有効期限付きセッション
- CSRF対策とセキュリティヘッダー
- 論理無効化を前提とした利用者モデル
- 組織設定
- 変更・ログイン監査ログ
- Goテンプレートによる制作プレビュー
- 仕入伝票の下書き・確定
- 仕入確定時の商品自動生成
- `YYYYMMDD + 3桁連番`の商品コード採番
- 確定再送時の二重生成防止
- 商品単品登録と簡易仕入伝票
- 在庫一覧、検索、状態フィルタ、商品詳細
- シリアル重複候補と理由付き続行
- 商品画像（JPEG・PNG・WebP、最大10枚）
- 在庫状態履歴
- 相場情報の手入力・CSV一括取込
- CSV全行検証・エラー表示・確定前プレビュー
- USD/JPY為替レートの履歴保存
- 参考相場の円換算、粗利額・粗利率の自動計算
- 売上伝票の下書き・確定・取消
- 売上確定時の通貨・為替・換算金額スナップショット
- 出荷伝票の下書き・確定・取消
- 出荷・返品の配送会社、追跡番号の保存・後編集
- 売上と出荷の独立登録、明細関連付け
- 複数回出荷、分納残数、累計超過出荷の防止
- 商品の公開・非公開設定
- ゲスト向け公開カタログと購入依頼
- 同一商品への複数未承認依頼
- 社内承認時だけ行う排他的な取置
- 取置期限切れ・却下・取消時の自動解除
- 売上確定時の取置完了
- 在庫・相場・仕入・売上・出荷・返品・棚卸・伝票履歴CSV
- 印刷・CSVダウンロードの発行者、日時、形式、保存ドライバ履歴
- PC・タブレット・スマートフォン対応の基準デザイン統合画面

## ローカルプレビュー

Go 1.26以降を使用します。Windowsでは次を実行します。

```powershell
.\scripts\dev.ps1
```

PostgreSQLを正本として起動する場合（推奨）:

```powershell
.\scripts\dev-postgres.ps1
```

8080番ポートを別プロセスが使用中なら、次のように別ポートを指定できます。

```powershell
.\scripts\dev-postgres.ps1 -Port 18086
```

起動後、指定したポートの`http://127.0.0.1:<ポート>/app/`を開きます。

相場CSVのサンプルは`examples/market_prices.csv`です。

プレビュー用アカウント：

- 管理者：`admin` / `preview-admin-2026`
- 作業者：`worker` / `preview-worker-2026`
- ゲスト：`G001` / `preview-guest-2026`

プレビュー用認証情報は開発環境だけで生成されます。本番では環境変数と初期管理者登録手順を使用します。

## 検証

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-check.ps1
```

## 資料

- [相場取込から売上確定までの実務操作（管理者・作業者／Mermaid図）](docs/operations-workflow.md)
- [目標アーキテクチャと段階移行メモ](docs/target-architecture.md)
- [REST API v1](docs/rest-api.md)
- [参照管理画面の固定仕様](docs/reference-admin.md)
- [フェーズ0現状監査](docs/phase-0-audit.md)
- [権限表](docs/permissions.md)
- [ルート一覧](docs/routes.md)
- [フェーズ2実装報告](docs/phase-2-inventory.md)
- [フェーズ3実装報告](docs/phase-3-market.md)
- [フェーズ4実装報告](docs/phase-4-sales-shipments.md)
- [フェーズ5実装報告](docs/phase-5-requests-reservations.md)
- [フェーズ6実装報告](docs/phase-6-approvals.md)
- [フェーズ7実装報告](docs/phase-7-usability.md)
- [フェーズ8実装報告](docs/phase-8-release-quality.md)
- [本番運用・バックアップ・復元](docs/operations.md)
- [設定項目](docs/configuration.md)
- [既知の制約](docs/known-issues.md)
