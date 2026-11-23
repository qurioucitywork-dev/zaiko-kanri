# Cloudflare・Neon 検証記録（2026-11-24）

## 対象と保護方針

- 作業用コピー: `C:\Work\zaiko-cloudflare-neon`。元の `C:\Work\zaiko-kanri` と既存 Docker データは変更していません。
- GitHub: `codex/cloudflare-neon-deploy`。`main` は変更していません。ドラフト PR: https://github.com/qurioucitywork-dev/zaiko-kanri/pull/2
- クラウド側は新規の空の Neon データベースです。既存商品の情報・伝票・画像はまだ移行していません。
- ログインと基本 API の疎通確認は、既存業務データを使用した全業務の同等性検証を意味しません。

## 成功した確認

| 対象 | 結果 |
| --- | --- |
| Go コンパイル | `go build ./...` 成功 |
| Go 既定テスト | Linux Docker / Go 1.26 の `go test ./...` が全パッケージ成功。専用接続情報が必要な PostgreSQL 結合テストはスキップ |
| フロントエンド生成 | `pnpm build` 成功 |
| フロントエンド回帰 | 29 ファイル成功。初回 27 件成功後、テスト読込・期待値を修正した 2 件を再実行して成功 |
| Neon 接続設定 | 既存 Docker の既定動作、直接接続によるマイグレーション、Neon 接続プールの誤使用防止のテスト成功 |
| R2 保存アダプター | 認証、キーのエスケープ、読込・保存・削除、危険なパスの拒否、認証情報を伴うリダイレクトの禁止を検証 |
| 公開前検査 | ステージ済み 356 ファイルを検査。実認証情報、DB ファイル、実行ログは含めず。Neon URL 検出はテスト用の架空値のみ |

配備担当が実際の Cloudflare / Neon / R2 環境で確認した内容:

- 管理者ログイン: HTTP 200。
- ログイン後の products / dashboard / me / notifications: HTTP 200。
- 未ログインの products: HTTP 401。
- 一般公開経路からの内部ストレージアクセス: HTTP 404。
- R2 の保存・読込・削除の往復確認: 成功。

## PostgreSQL 18 の追加結合テスト

実アプリとは別の使い捨てコンテナ・プライベート Docker ネットワーク・28 個の専用 DB を使用しました。ホストへのポート公開や既存 DB の変更は行っていません。終了後にテストコンテナと専用ネットワークを停止・削除しています。

最初の追加実行は **303 成功・16 スキップ・18 失敗イベント** でした。数は Go JSON のサブテストも含みます。この中の静的画面 SHA 検証 1 件は、既存ソースに対して古かった期待値を更新し、最終の既定テストで成功しました。

オークション手数料、99999 の在庫非干渉調整行、SMBC 取得・通知、委託修正、付属品・ベルト表示、返品差戻し再申請、日次採番、ゲスト認証などは、この PostgreSQL 18 の実行でも成功しました。

残った失敗は以下の通りです。テスト用初期値・仮テーブルの不足等が観測されましたが、改修前の同条件実行との比較は未実施のため、すべてを既存不具合と断定していません。

| テスト名 | 観測された失敗 |
| --- | --- |
| `TestCashPurchasePaymentPostgres` | スタッフ情報が見つからない |
| `TestCostMergeConfirmationPostgres` | スタッフ情報が見つからない |
| `TestCostMergeInventorySyntheticAudit` | スタッフ情報が見つからない。JPY サブテストも失敗 |
| `TestCostMergeOptionalAudit` | スタッフ情報が見つからない |
| `TestDashboardProductFactsPostgres` | テスト商品と仕入伝票の仕入日が一致せず DB 制約違反 |
| `TestGuestCatalogHidesPurchasingUntilReceived` | 仮テーブルに `region_type` 列がない |
| `TestLinkageRoundTripPostgres` | スタッフ情報が見つからない |
| `TestProductWeightPostgres` | 共有テスト DB の付属品コードが重複 |
| `TestPurchaseCostStatusPostgres` | スタッフ情報が見つからない |
| `TestForeignPurchaseRevisionPostgres` | USD / HKD サブテストのスタッフ情報が見つからない |
| `TestReturnedSaleBuyerLockedPostgres` | 共有テスト DB の為替スナップショット ID が重複 |
| `TestDirectConsignmentSalePostgres` | スタッフ情報が見つからない |
| `TestRepeatedSerialRegistrationPostgres` | スタッフ情報が見つからない |
| `TestShipmentEnteredPricePostgres` | 仮テーブルに日次採番テーブルがない |

スキップには、既存商品を含む専用フィクスチャが必要な原価調整・原価合流テストと、今回接続設定を与えていない Web 結合テストが含まれます。

## 今回修正したテスト側の整合性

業務ロジックは変更せず、検証側だけを現行画面に合わせました。

- Windows の CRLF 改行で関数の切り出し範囲が壊れないようにする。
- 円統一 CSV の `原価（JPY）` ヘッダーを期待する。
- 実画面が読み込むオークション手数料・返品調整スクリプトを DOM テストでも読み込む。
- バージョン付きゲストスクリプト URL を許容する。
- 現行 `app.js` のハッシュを画面整合性テストに反映する。

## 実務テスト開始前の残件

1. 上記の専用フィクスチャを修正し、PostgreSQL 結合テストを再実行する。
2. 既存データ・画像を保全してから、クラウドの検証用データとして移行し、件数と金額を照合する。
3. 仕入、出荷、売上、返品、原価調整、手数料、CSV、PDF、タグを実際のクラウド画面で通し確認する。
4. 複数人操作、コンテナ再起動、DB 接続断、画像保持、バックアップからの復元を確認する。
5. 社内回線・外出先 VPN・ゲスト別アクセスの方針を実環境に設定し、許可・拒否の両方を確認する。

この段階はクラウド上の実務テスト環境の構築・基本疎通確認までであり、本番切替や既存データ移行の完了ではありません。

## 既存データの所在調査

既存構成は PostgreSQL `zaiko_clean` と Docker ボリューム `zaiko-clean_postgres-clean-data` を参照します。ただし、今回起動した Docker Engine には、このボリュームと旧アプリのコンテナが見つかりませんでした。今回の作業では旧ボリューム・旧データの削除や初期化は行っていません。

旧フォルダーには画像 28 ファイル（約 12.5 MB）、互換用 SQLite、PostgreSQL の 10 月 11 日および 10 月 15 日付バックアップが残っています。10 月 15 日分が最新であることは確認できていないため、自動復元は行いませんでした。旧 Docker の保存領域または採用する正式なバックアップを特定し、商品・伝票・金額・画像を照合してから新環境へ移行する必要があります。

新環境の管理者資格情報は専用に生成しました。秘密情報は Git 対象外のローカル `.runtime` と Cloudflare Secrets に保管し、ローカル `.runtime` の ACL は実行ユーザーと SYSTEM に限定しています。
