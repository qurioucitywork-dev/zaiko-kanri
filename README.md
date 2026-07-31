# 在庫管理システム

時計・貴金属等の仕入、在庫、相場、売上、出荷、購入依頼、承認、監査を一元管理するGo製業務システムです。

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
- 商品登録と簡易仕入伝票
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
- 売上と出荷の独立登録、明細関連付け
- 複数回出荷、分納残数、累計超過出荷の防止
- 商品の公開・非公開設定
- ゲスト向け公開カタログと購入依頼
- 同一商品への複数未承認依頼
- 社内承認時だけ行う排他的な取置
- 取置期限切れ・却下・取消時の自動解除
- 売上確定時の取置完了

## ローカルプレビュー

Go 1.26以降を使用します。Windowsでは次を実行します。

```powershell
.\scripts\dev.ps1
```

起動後、`http://127.0.0.1:8080`を開きます。

相場CSVのサンプルは`examples/market_prices.csv`です。

プレビュー用アカウント：

- 管理者：`admin` / `preview-admin-2026`
- 作業者：`worker` / `preview-worker-2026`

プレビュー用認証情報は開発環境だけで生成されます。本番では環境変数と初期管理者登録手順を使用します。

## 検証

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-check.ps1
```

## 資料

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
